"""Authenticated console API for grouped logical profiles.

No game operations are exposed. Mutations use existing write guard, bounded
selection, previews, explicit confirmation and server-side audit records.
"""
from __future__ import annotations

from fastapi import APIRouter, Depends, HTTPException, Query
from pydantic import BaseModel, ConfigDict, Field

from reqngo.fleet_policy import FleetPolicyConsole


class NewGroup(BaseModel):
    model_config = ConfigDict(extra="forbid")
    group_key: str = Field(min_length=2, max_length=40)
    label: str = Field(min_length=2, max_length=60)
    description: str = Field(default="", max_length=240)


class Assignment(BaseModel):
    model_config = ConfigDict(extra="forbid")
    profile_keys: list[str] = Field(min_length=1, max_length=200)
    group_key: str | None = Field(default=None, max_length=40)


class BulkAction(BaseModel):
    model_config = ConfigDict(extra="forbid")
    enabled: bool
    confirmation: str = Field(min_length=8, max_length=60)
    expected_count: int = Field(ge=0, le=10_000_000)


class PolicyUpdate(BaseModel):
    model_config = ConfigDict(extra="forbid")
    preferred_worker: str | None = Field(default=None, max_length=64)
    probe_interval_seconds: int | None = Field(default=None, ge=30, le=3600)


class DiagnosticRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")
    probe_mode: str = Field(default="storage", pattern="^(storage|android)$")


def router_for(console: FleetPolicyConsole, read_guard, write_guard):
    router = APIRouter(prefix="/api/fleet/console")

    def apply(callback):
        try:
            return callback()
        except ValueError as exc:
            raise HTTPException(422, str(exc)) from exc

    @router.get("/groups")
    def groups(_: bool = Depends(read_guard)):
        return {"items": console.groups()}

    @router.post("/groups")
    def create(body: NewGroup, _: bool = Depends(write_guard)):
        return apply(lambda: console.create_group(
            body.group_key, body.label, body.description))

    @router.get("/profiles")
    def profiles(q: str = Query(default="", max_length=40),
                 group: str = Query(default="all", max_length=40),
                 enabled: str = Query(default="all"),
                 offset: int = Query(default=0, ge=0, le=10_000_000),
                 limit: int = Query(default=50, ge=1, le=100),
                 _: bool = Depends(read_guard)):
        return apply(lambda: console.profiles(q=q, group=group,
                    enabled=enabled, offset=offset, limit=limit))

    @router.post("/assign")
    def assign(body: Assignment, _: bool = Depends(write_guard)):
        return apply(lambda: console.assign(body.profile_keys, body.group_key))

    @router.get("/groups/{key}/preview")
    def preview(key: str, enabled: bool = Query(...),
                _: bool = Depends(read_guard)):
        return apply(lambda: console.preview(key, enabled))

    @router.post("/groups/{key}/enabled")
    def bulk(key: str, body: BulkAction, _: bool = Depends(write_guard)):
        return apply(lambda: console.bulk_set_enabled(
            key, body.enabled, body.confirmation, body.expected_count))

    @router.get("/groups/{key}/policy")
    def get_group_policy(key: str, _: bool = Depends(read_guard)):
        return apply(lambda: console.group_policy(key))

    @router.put("/groups/{key}/policy")
    def set_group_policy(key: str, body: PolicyUpdate,
                         _: bool = Depends(write_guard)):
        if body.probe_interval_seconds is None:
            raise HTTPException(422, "L'intervalle du groupe doit etre explicite.")
        return apply(lambda: console.set_group_policy(
            key, body.preferred_worker, body.probe_interval_seconds))

    @router.get("/profiles/{key}/detail")
    def profile_detail(key: str, _: bool = Depends(read_guard)):
        return apply(lambda: console.profile_detail(key))

    @router.put("/profiles/{key}/override")
    def profile_override(key: str, body: PolicyUpdate,
                         _: bool = Depends(write_guard)):
        return apply(lambda: console.set_profile_override(
            key, body.preferred_worker, body.probe_interval_seconds))

    @router.post("/profiles/{key}/diagnostic")
    def profile_diagnostic(key: str, body: DiagnosticRequest,
                           _: bool = Depends(write_guard)):
        return apply(lambda: console.enqueue_profile_diagnostic(
            key, body.probe_mode))

    @router.get("/audit")
    def audit(limit: int = Query(default=25, ge=1, le=100),
              _: bool = Depends(read_guard)):
        return console.audit(limit)

    return router
