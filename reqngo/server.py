"""Authenticated web API for the AI Guardian simulation.

Single-process pilot only. Bind to localhost; place behind authenticated HTTPS
or a VPN if deployed. No Supercell connections or emulator control.
"""
from __future__ import annotations

import hmac
import os
import threading
import time
from contextlib import asynccontextmanager
from pathlib import Path

from fastapi import Depends, FastAPI, HTTPException, Request
from fastapi.responses import FileResponse
from fastapi.security import HTTPBasic, HTTPBasicCredentials
from pydantic import BaseModel

from reqngo.core import Guardian
from reqngo.telemetry import TelemetryStore
from reqngo.telemetry_api import router_for

BASE = Path(__file__).resolve().parent
DATA = Path(os.getenv("GUARDIAN_DATA_DIR", BASE.parent / "data"))
engine = Guardian(DATA / "guardian.sqlite3")
telemetry = TelemetryStore(DATA / "telemetry.sqlite3")
stop = threading.Event()
security = HTTPBasic()


def guard(credentials: HTTPBasicCredentials = Depends(security)):
    password = os.getenv("GUARDIAN_PASSWORD", "")
    if not password or len(password) < 16:
        raise HTTPException(503, "GUARDIAN_PASSWORD requis (16 caractères minimum).")
    valid = (hmac.compare_digest(credentials.username.encode(), b"admin")
             and hmac.compare_digest(credentials.password.encode(), password.encode()))
    if not valid:
        raise HTTPException(401, "Identifiants incorrects",
                            headers={"WWW-Authenticate": 'Basic realm="Guardian"'})
    return True


def write_guard(request: Request, _: bool = Depends(guard)):
    if request.headers.get("X-Guardian-Action") != "1":
        raise HTTPException(403, "En-tête de modification manquant.")
    origin = request.headers.get("origin")
    if origin:
        host = request.headers.get("host", "")
        if origin != str(request.url.scheme) + "://" + host:
            raise HTTPException(403, "Origine non autorisée.")
    return True


def supervise():
    while not stop.wait(2):
        try:
            cfg = engine.settings()
            due = time.time() - cfg["last_full_scan"] >= cfg["scan_interval_seconds"]
            engine.scan(full=due)
            if due:
                telemetry.check_stale()
        except Exception as exc:
            # A crash in one scan must not kill the scheduler.
            print("Guardian supervisor:", type(exc).__name__, flush=True)


@asynccontextmanager
async def lifespan(app):
    stop.clear()
    worker = threading.Thread(target=supervise, daemon=True, name="guardian-monitor")
    worker.start()
    try:
        yield
    finally:
        stop.set()
        worker.join(timeout=5)
        engine.close()
        telemetry.close()


app = FastAPI(title="REQ N GO Guardian", docs_url=None, redoc_url=None,
              openapi_url=None, lifespan=lifespan)
app.include_router(router_for(telemetry, guard))


@app.middleware("http")
async def security_headers(request: Request, call_next):
    response = await call_next(request)
    response.headers["Cache-Control"] = "no-store"
    response.headers["X-Content-Type-Options"] = "nosniff"
    response.headers["X-Frame-Options"] = "DENY"
    response.headers["Referrer-Policy"] = "no-referrer"
    response.headers["Content-Security-Policy"] = (
        "default-src 'self'; script-src 'self' 'unsafe-inline'; "
        "style-src 'self' 'unsafe-inline'; img-src 'self'; "
        "connect-src 'self'; object-src 'none'; frame-ancestors 'none'"
    )
    return response


@app.get("/")
def dashboard(_: bool = Depends(guard)):
    return FileResponse(BASE / "static" / "index.html")


@app.get("/api/state")
def state(_: bool = Depends(guard)):
    return engine.diagnostics()


class Config(BaseModel):
    scan_interval_seconds: int | None = None
    auto_repair: bool | None = None
    retry_delay_seconds: int | None = None
    max_attempts: int | None = None
    pilot_mode: bool | None = None


@app.post("/api/settings")
def settings(body: Config, _: bool = Depends(write_guard)):
    try:
        return engine.configure(body.model_dump(exclude_none=True))
    except ValueError as exc:
        raise HTTPException(422, str(exc))


class Pilots(BaseModel):
    ids: list[int]


@app.post("/api/pilots")
def pilots(body: Pilots, _: bool = Depends(write_guard)):
    try:
        return {"ids": engine.set_pilots(body.ids)}
    except ValueError as exc:
        raise HTTPException(422, str(exc))


class Enabled(BaseModel):
    enabled: bool


@app.post("/api/accounts/{account_id}/enabled")
def enabled(account_id: int, body: Enabled, _: bool = Depends(write_guard)):
    try:
        engine.enable(account_id, body.enabled)
        return {"ok": True}
    except ValueError as exc:
        raise HTTPException(422, str(exc))


class Fault(BaseModel):
    account_id: int
    code: str


@app.post("/api/demo/fault")
def fault(body: Fault, _: bool = Depends(write_guard)):
    try:
        return engine.inject(body.account_id, body.code)
    except ValueError as exc:
        raise HTTPException(422, str(exc))


@app.post("/api/scan")
def scan(_: bool = Depends(write_guard)):
    result = engine.scan(full=True)
    result["telemetry_alerts"] = telemetry.check_stale()["alerts_created"]
    return result


@app.post("/api/incidents/{incident_id}/resolve")
def resolve(incident_id: int, _: bool = Depends(write_guard)):
    try:
        engine.resolve(incident_id)
        return {"ok": True}
    except ValueError as exc:
        raise HTTPException(422, str(exc))
