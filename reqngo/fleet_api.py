"""Authenticated read-only fleet inventory and creation of diagnostic test work.

Worker leases are intentionally NOT exposed over HTTP until workers have
individual identities, narrowly scoped credentials and audit logs.
"""
from __future__ import annotations

import hmac
import json
import os

from fastapi import APIRouter, Depends, HTTPException, Query, Request
from pydantic import BaseModel, ConfigDict, Field

from reqngo.fleet import FleetStore, _WORKER


class Batch(BaseModel):
    model_config = ConfigDict(extra="forbid")
    prefix: str = Field(min_length=1, max_length=24)
    start: int = Field(ge=1, le=9_999_999)
    count: int = Field(ge=1, le=10_000)


class Probe(BaseModel):
    model_config = ConfigDict(extra="forbid")
    profile_key: str = Field(min_length=1, max_length=40)
    dedupe_key: str = Field(min_length=8, max_length=80)


class Enabled(BaseModel):
    model_config = ConfigDict(extra="forbid")
    enabled: bool


class WorkerHeartbeat(BaseModel):
    model_config = ConfigDict(extra="forbid")
    status: str = Field(pattern="^(ok|busy|degraded)$")
    capacity: int = Field(default=1, ge=1, le=8)


class WorkerClaim(BaseModel):
    model_config = ConfigDict(extra="forbid")
    limit: int = Field(default=1, ge=1, le=8)
    capacity: int = Field(default=1, ge=1, le=8)
    lease_seconds: int = Field(default=90, ge=15, le=3600)


class WorkerLease(BaseModel):
    model_config = ConfigDict(extra="forbid")
    job_id: int = Field(gt=0)
    lease_token: str = Field(min_length=32, max_length=80)
    lease_seconds: int = Field(default=90, ge=15, le=3600)


class WorkerCompletion(BaseModel):
    model_config = ConfigDict(extra="forbid")
    job_id: int = Field(gt=0)
    lease_token: str = Field(min_length=32, max_length=80)
    success: bool


def authorized_worker(request: Request):
    """Bind each worker ID to its own key; fail closed without configuration."""
    name = request.headers.get("X-Fleet-Worker-Id", "")
    if not _WORKER.fullmatch(name):
        raise HTTPException(401, "Identifiants de worker invalides.")
    raw = os.getenv("FLEET_WORKER_TOKENS_JSON", "")
    try:
        config = json.loads(raw)
        if (not isinstance(config, dict) or not 1 <= len(config) <= 512
                or any(not isinstance(k, str) or not _WORKER.fullmatch(k)
                       or not isinstance(v, str) or len(v) < 32
                       for k, v in config.items())
                or len(set(config.values())) != len(config)):
            raise ValueError("Bad configuration")
    except (ValueError, TypeError):
        raise HTTPException(503, "Authentification des workers non configuree.")
    expected = config.get(name)
    header = request.headers.get("Authorization", "")
    supplied = header[7:] if header.startswith("Bearer ") else ""
    if not expected or not supplied or not hmac.compare_digest(
            expected.encode(), supplied.encode()):
        raise HTTPException(401, "Identifiants de worker invalides.")
    return name


def router_for(store: FleetStore, read_guard, write_guard):
    router = APIRouter()

    @router.get("/api/fleet/summary")
    def summary(_: bool = Depends(read_guard)):
        return store.summary()

    @router.get("/api/fleet/workers")
    def worker_list(limit: int = Query(default=100, ge=1, le=200),
                    _: bool = Depends(read_guard)):
        return {"items": store.workers(limit)}

    @router.post("/api/fleet/worker/heartbeat")
    def worker_heartbeat(body: WorkerHeartbeat, worker_id: str = Depends(authorized_worker)):
        try:
            return store.heartbeat(worker_id, body.status, body.capacity)
        except ValueError as exc:
            raise HTTPException(422, str(exc))

    @router.post("/api/fleet/worker/claim")
    def worker_claim(body: WorkerClaim, worker_id: str = Depends(authorized_worker)):
        try:
            return {"items": store.claim(worker_id, limit=body.limit,
                    capacity=body.capacity, lease_seconds=body.lease_seconds)}
        except ValueError as exc:
            raise HTTPException(422, str(exc))

    @router.post("/api/fleet/worker/renew")
    def worker_renew(body: WorkerLease, worker_id: str = Depends(authorized_worker)):
        try:
            return store.renew(body.job_id, worker_id, body.lease_token,
                               lease_seconds=body.lease_seconds)
        except ValueError as exc:
            raise HTTPException(409, str(exc))

    @router.post("/api/fleet/worker/complete")
    def worker_complete(body: WorkerCompletion, worker_id: str = Depends(authorized_worker)):
        try:
            return store.complete(body.job_id, worker_id, body.lease_token, body.success)
        except ValueError as exc:
            raise HTTPException(409, str(exc))

    @router.get("/api/fleet/profiles")
    def profiles(offset: int = Query(default=0, ge=0, le=10_000_000),
                 limit: int = Query(default=100, ge=1, le=200),
                 _: bool = Depends(read_guard)):
        return {"items": store.profiles(offset, limit), "offset": offset, "limit": limit}

    @router.post("/api/fleet/profiles/batch")
    def register(body: Batch, _: bool = Depends(write_guard)):
        try:
            return store.register_batch(body.prefix, body.start, body.count)
        except ValueError as exc:
            raise HTTPException(status_code=422, detail=str(exc))

    @router.post("/api/fleet/profiles/{profile_key}/enabled")
    def enabled(profile_key: str, body: Enabled, _: bool = Depends(write_guard)):
        try:
            store.set_enabled(profile_key, body.enabled)
        except ValueError as exc:
            raise HTTPException(status_code=422, detail=str(exc))
        return {"ok": True}

    @router.post("/api/fleet/probes")
    def enqueue(body: Probe, _: bool = Depends(write_guard)):
        try:
            return store.enqueue_probe(body.profile_key, body.dedupe_key)
        except ValueError as exc:
            raise HTTPException(status_code=422, detail=str(exc))

    return router
