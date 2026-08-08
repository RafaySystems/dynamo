// SPDX-FileCopyrightText: Copyright (c) 2025-2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

//! Advisory prefill-worker load booking.
//!
//! On the EPP path, prefill selection uses [`PrefillRouter::query_prefill_worker`], which
//! is advisory and books no scheduler state. Without booking, every prefill worker looks
//! equally loaded to the selector, so a worker busy with a large prompt is as likely to be
//! chosen as an idle one and new requests queue behind it.
//!
//! These methods book the selected worker's prefill load as a separate step after
//! selection, and release it at prefill completion. The load accrues via the scheduler's
//! `prefill_load_hint`, which is independent of `router_track_active_blocks`. Booking here
//! is non-atomic with selection (a concurrent caller may select the same worker before the
//! book lands); tightening that into an atomic admission is a tracked follow-up.

use anyhow::Result;
use dynamo_kv_router::protocols::WorkerWithDpRank;

use super::{InnerPrefillRouter, PrefillRouter};

impl PrefillRouter {
    /// Book prefill load for `worker` so subsequent selections see it as busier.
    ///
    /// Books only the uncached prefill work (ISL minus this worker's local cache overlap).
    /// No-op when the prefill router is inactive or is a non-KV (round-robin) router, which
    /// carries no per-worker load state. Paired release is [`PrefillRouter::free`].
    pub async fn add_request(
        &self,
        request_id: String,
        tokens: &[u32],
        worker: WorkerWithDpRank,
    ) -> Result<()> {
        let Some(InnerPrefillRouter::KvRouter(router)) = self.prefill_router.get() else {
            return Ok(());
        };
        let chooser = &router.chooser;

        let overlap_blocks = chooser
            .get_overlap_blocks(tokens, None, worker, None)
            .await
            .map_err(|e| anyhow::anyhow!("prefill get_overlap_blocks failed: {e:?}"))?;
        let cached_tokens = overlap_blocks as usize * chooser.block_size() as usize;

        chooser
            .add_request(
                request_id,
                tokens,
                None,
                cached_tokens,
                None,
                worker,
                None,
                None,
            )
            .await;
        Ok(())
    }

    /// Release a booking made by [`PrefillRouter::add_request`].
    ///
    /// No-op when the prefill router is inactive or is a non-KV router. Unknown request IDs
    /// surface as an error from the scheduler; callers treat release as best-effort.
    pub async fn free(&self, request_id: &str) -> Result<()> {
        let Some(InnerPrefillRouter::KvRouter(router)) = self.prefill_router.get() else {
            return Ok(());
        };
        router
            .chooser
            .free(request_id)
            .await
            .map_err(|e| anyhow::anyhow!("prefill free failed: {e}"))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::discovery::ModelManager;
    use dynamo_kv_router::protocols::WorkerWithDpRank;
    use dynamo_runtime::pipeline::RouterMode;
    use std::sync::Arc;

    // A router that was never activated resolves to no inner router; booking/free must be
    // harmless no-ops (never panic, never error) so the EPP path can call them
    // unconditionally before prefill workers are discovered.
    #[tokio::test]
    async fn booking_is_noop_when_router_inactive() {
        let router = PrefillRouter::disabled(
            Arc::new(ModelManager::new()),
            RouterMode::RoundRobin,
            None,
        );
        let worker = WorkerWithDpRank::new(1, 0);

        router
            .add_request("req-1".to_string(), &[1, 2, 3, 4], worker)
            .await
            .expect("add_request must be a no-op when inactive");
        router
            .free("req-1")
            .await
            .expect("free must be a no-op when inactive");
    }
}
