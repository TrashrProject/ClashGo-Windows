"""Authenticated read-only fleet inventory and creation of diagnostic test work.

Worker leases are intentionally NOT exposed over HTTP until workers have
individual identities, narrowly scoped credentials and audit logs.
"""
from __future__ import annotations

from fastapi import APIRouter, Depends, HTTPException, Query
from pydantic import BaseModel, ConfigDict, Field

from reqngo.fleet import FleetStore


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


def router_for(store: FleetStore, read_guard, write_guard):
    router = APIRouter()

    @router.get("/api/fleet/summary")
    def summary(_: bool = Depends(read_guard)):
        return store.summary()

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
