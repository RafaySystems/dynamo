/*
Copyright 2025 NVIDIA Corporation.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package disagg

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"

	log "sigs.k8s.io/controller-runtime/pkg/log"
	logutil "sigs.k8s.io/gateway-api-inference-extension/pkg/common/observability/logging"
	fwkdl "sigs.k8s.io/gateway-api-inference-extension/pkg/epp/framework/interface/datalayer"
	plugins "sigs.k8s.io/gateway-api-inference-extension/pkg/epp/framework/interface/plugin"
	rc "sigs.k8s.io/gateway-api-inference-extension/pkg/epp/framework/interface/requestcontrol"
	schedtypes "sigs.k8s.io/gateway-api-inference-extension/pkg/epp/framework/interface/scheduling"

	dynscorer "github.com/nvidia/dynamo/deploy/inference-gateway/pkg/plugins/dynamo_kv_scorer"
)

const (
	// DynPrefillScorerType is the plugin type registered in the plugin registry.
	DynPrefillScorerType = "dyn-prefill-scorer"

	prefillStateKey = "dynamo-prefill-routing-state"
)

// compile-time type assertions
var _ schedtypes.Scorer = &DynPrefillScorer{}
var _ rc.PreRequest = &DynPrefillScorer{}
var _ rc.ResponseBodyProcessor = &DynPrefillScorer{}

// PrefillRoutingState holds routing information passed from Score() to PreRequest()
// so the selected prefill worker's load can be booked, and released in ResponseBody().
type PrefillRoutingState struct {
	WorkerID  uint64
	DpRank    uint32
	TokenData []int64
}

// Clone implements plugins.StateData.
func (s *PrefillRoutingState) Clone() plugins.StateData {
	if s == nil {
		return nil
	}
	clone := &PrefillRoutingState{
		WorkerID: s.WorkerID,
		DpRank:   s.DpRank,
	}
	if s.TokenData != nil {
		clone.TokenData = make([]int64, len(s.TokenData))
		copy(clone.TokenData, s.TokenData)
	}
	return clone
}

// DynPrefillScorerConfig holds the configuration for the DynPrefillScorer plugin.
type DynPrefillScorerConfig struct{}

// DynPrefillScorerFactory defines the factory function for DynPrefillScorer.
func DynPrefillScorerFactory(name string, rawParameters json.RawMessage, handle plugins.Handle) (plugins.Plugin, error) {
	cfg := DynPrefillScorerConfig{}
	if rawParameters != nil {
		if err := json.Unmarshal(rawParameters, &cfg); err != nil {
			return nil, fmt.Errorf("failed to parse %s plugin parameters: %w", DynPrefillScorerType, err)
		}
	}

	if err := dynscorer.InitFFI(); err != nil {
		return nil, fmt.Errorf("Dynamo FFI init for prefill scorer failed: %w", err)
	}

	return NewDynPrefillScorer(handle.Context()).WithName(name), nil
}

// NewDynPrefillScorer initializes a new DynPrefillScorer.
func NewDynPrefillScorer(ctx context.Context) *DynPrefillScorer {
	return &DynPrefillScorer{
		typedName:   plugins.TypedName{Type: DynPrefillScorerType, Name: DynPrefillScorerType},
		pluginState: plugins.NewPluginState(ctx),
	}
}

// DynPrefillScorer is a scorer plugin for the prefill scheduling profile.
type DynPrefillScorer struct {
	typedName      plugins.TypedName
	pluginState    *plugins.PluginState
	firstTokenSeen sync.Map
}

// TypedName returns the type and name tuple of this plugin instance.
func (s *DynPrefillScorer) TypedName() plugins.TypedName {
	return s.typedName
}

// WithName sets the name of the scorer.
func (s *DynPrefillScorer) WithName(name string) *DynPrefillScorer {
	s.typedName.Name = name
	return s
}

// Category returns the scorer category.
func (s *DynPrefillScorer) Category() schedtypes.ScorerCategory {
	return schedtypes.Affinity
}

// Score scores endpoints for prefill suitability.
func (s *DynPrefillScorer) Score(ctx context.Context, cycleState *schedtypes.CycleState, req *schedtypes.InferenceRequest, endpoints []schedtypes.Endpoint) map[schedtypes.Endpoint]float64 {
	logger := log.FromContext(ctx)

	if !readPrefillEnabled(cycleState) {
		logger.V(logutil.VERBOSE).Info("DynPrefillScorer: prefill not enabled, returning zero scores")
		return uniformScores(endpoints, 0)
	}

	requestJSON, err := buildRequestJSON(req)
	if err != nil {
		logger.V(logutil.DEFAULT).Error(err, "DynPrefillScorer: failed to build request")
		return uniformScores(endpoints, 0)
	}

	endpointsJSON := serializeEndpoints(endpoints)
	logger.V(logutil.DEFAULT).Info("DynPrefillScorer: endpoints received for scoring",
		"endpointCount", len(endpoints),
		"endpointsJSON", string(endpointsJSON))

	result, err := dynscorer.CallRoutePrefillRequest(requestJSON, endpointsJSON)
	if err != nil {
		logger.V(logutil.DEFAULT).Error(err, "DynPrefillScorer: FFI prefill routing failed")
		cycleState.Write(PrefillEnabledStateKey, &PrefillEnabledState{Enabled: false})
		return uniformScores(endpoints, 0)
	}

	prefillWorkerID := strconv.FormatUint(result.WorkerID, 10)
	logger.V(logutil.DEFAULT).Info("DynPrefillScorer: prefill worker selected",
		"prefillWorkerID", prefillWorkerID,
		"prefillDpRank", result.DpRank,
		"tokenCount", len(result.TokenData))

	if req.Headers == nil {
		req.Headers = map[string]string{}
	}
	req.Headers[PrefillWorkerIDHeader] = prefillWorkerID
	if result.DpRank != dynscorer.UnsetDpRank {
		req.Headers[PrefillDpRankHeader] = strconv.FormatUint(uint64(result.DpRank), 10)
	} else {
		delete(req.Headers, PrefillDpRankHeader)
	}

	// Store routing state so PreRequest can book the selected prefill worker's load
	// and ResponseBody can release it at prefill completion.
	if req.RequestId != "" {
		dpRank := result.DpRank
		if dpRank == dynscorer.UnsetDpRank {
			dpRank = 0
		}
		s.pluginState.Write(req.RequestId, plugins.StateKey(prefillStateKey), &PrefillRoutingState{
			WorkerID:  result.WorkerID,
			DpRank:    dpRank,
			TokenData: result.TokenData,
		})
	}

	return uniformScores(endpoints, 1.0)
}

// PreRequest books the selected prefill worker's load so subsequent prefill
// selections see it as busier. The booking is released in ResponseBody at prefill
// completion. Booking failures are logged and non-fatal — routing still proceeds.
func (s *DynPrefillScorer) PreRequest(ctx context.Context, request *schedtypes.InferenceRequest, _ *schedtypes.SchedulingResult) {
	logger := log.FromContext(ctx)

	if request == nil || request.RequestId == "" {
		logger.V(logutil.DEBUG).Info("DynPrefillScorer PreRequest: no request ID, skipping")
		return
	}

	state, err := plugins.ReadPluginStateKey[*PrefillRoutingState](
		s.pluginState, request.RequestId, plugins.StateKey(prefillStateKey),
	)
	s.pluginState.Delete(request.RequestId)

	if err != nil {
		logger.V(logutil.DEBUG).Info("DynPrefillScorer PreRequest: no routing state found",
			"requestID", request.RequestId)
		return
	}

	if addErr := dynscorer.CallAddPrefillRequest(
		request.RequestId,
		state.TokenData,
		state.WorkerID,
		state.DpRank,
	); addErr != nil {
		logger.V(logutil.DEFAULT).Error(addErr, "DynPrefillScorer PreRequest: failed to book prefill request",
			"requestID", request.RequestId)
		return
	}

	logger.V(logutil.VERBOSE).Info("DynPrefillScorer PreRequest: booked prefill request",
		"requestID", request.RequestId,
		"workerID", state.WorkerID,
		"dpRank", state.DpRank,
		"tokenCount", len(state.TokenData))
}

// ResponseBody releases the prefill worker's booking at prefill completion.
//
// A prefill worker is done once the first output token appears (its KV has been
// handed off to the decode worker), so release on the first response event rather
// than end-of-stream — holding until EndOfStream would over-book the prefill worker
// for the entire decode duration. The LoadOrStore guard makes release fire exactly
// once, whether the first event is a token chunk or an empty end-of-stream.
func (s *DynPrefillScorer) ResponseBody(ctx context.Context, request *schedtypes.InferenceRequest, response *rc.Response, _ *fwkdl.EndpointMetadata) {
	if request == nil || request.RequestId == "" {
		return
	}

	logger := log.FromContext(ctx)

	if _, alreadySeen := s.firstTokenSeen.LoadOrStore(request.RequestId, true); !alreadySeen {
		if err := dynscorer.CallFreePrefillRequest(request.RequestId); err != nil {
			logger.V(logutil.DEFAULT).Error(err, "DynPrefillScorer ResponseBody: failed to free prefill request",
				"requestID", request.RequestId)
		} else {
			logger.V(logutil.VERBOSE).Info("DynPrefillScorer ResponseBody: released prefill booking",
				"requestID", request.RequestId)
		}
	}

	if response != nil && response.EndOfStream {
		s.firstTokenSeen.Delete(request.RequestId)
	}
}
