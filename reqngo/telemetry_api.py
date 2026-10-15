"""Authenticated ingestion API for diagnostics sent by owned test workers."""
from __future__ import annotations

import hmac
import os

from fastapi import APIRouter, Depends, HTTPException, Request
from fastapi.responses import JSONResponse
from pydantic import BaseModel, ConfigDict, Field

from reqngo.telemetry import TelemetryStore


class Heartbeat(BaseModel):
    model_config = ConfigDict(extra="forbid")
    agent_id: str = Field(max_length=40)
    status: str = "ok"
    version: str = "unknown"


class Report(BaseModel):
    model_config = ConfigDict(extra="forbid")
    agent_id: str = Field(max_length=40)
    event_key: str = Field(max_length=80)
    code: str = Field(max_length=64)
    level: str = "error"
    profile_id: int | None = None
    metadata: dict | None = None


def router_for(store: TelemetryStore, dashboard_guard):
    router = APIRouter()

    def agent_guard(request: Request):
        expected = os.getenv("GUARDIAN_AGENT_TOKEN", "")
        header = request.headers.get("authorization", "")
        supplied = header[7:] if header.startswith("Bearer ") else ""
        if len(expected) < 32:
            raise HTTPException(status_code=503, detail="Ingestion non configuree.")
        if not supplied or not hmac.compare_digest(supplied, expected):
            raise HTTPException(status_code=401, detail="Jeton agent invalide.")
        return True

    @router.post("/api/telemetry/heartbeat")
    def heartbeat(body: Heartbeat, _: bool = Depends(agent_guard)):
        try:
            return store.heartbeat(**body.model_dump())
        except ValueError as exc:
            raise HTTPException(422, str(exc))

    @router.post("/api/telemetry/report")
    def report(body: Report, _: bool = Depends(agent_guard)):
        try:
            return store.report(**body.model_dump())
        except ValueError as exc:
            raise HTTPException(422, str(exc))

    @router.get("/api/telemetry/state")
    def state(_: bool = Depends(dashboard_guard)):
        return store.snapshot()

    @router.get("/api/telemetry/export")
    def export(_: bool = Depends(dashboard_guard)):
        result = JSONResponse(content=store.snapshot(limit=200))
        result.headers["Content-Disposition"] = 'attachment; filename="guardian-diagnostics.json"'
        return result

    return router
