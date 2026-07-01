"""
Mock Oracle Database Middleware API
Simulates a middleware layer in front of an Oracle cluster.
Backed by SQLite via SQLAlchemy.
"""

import asyncio
import json
import os
import uuid
from datetime import datetime, timezone
from pathlib import Path
from typing import AsyncGenerator, Optional

from fastapi import BackgroundTasks, FastAPI, HTTPException, Request
from fastapi.responses import HTMLResponse, Response
from pydantic import BaseModel, Field
from sqlalchemy import Column, DateTime, Integer, String, Text, create_engine
from sqlalchemy.orm import DeclarativeBase, Session, sessionmaker
from sse_starlette.sse import EventSourceResponse

# ---------------------------------------------------------------------------
# Database setup
# ---------------------------------------------------------------------------

_db_url = os.getenv("DATABASE_URL", "sqlite:///./mock_oracle.db")
engine = create_engine(
    _db_url,
    connect_args={"check_same_thread": False},
)


class Base(DeclarativeBase):
    pass


class DatabaseRecord(Base):
    __tablename__ = "databases"

    id = Column(String, primary_key=True, default=lambda: str(uuid.uuid4()))
    db_name = Column(String, nullable=False)
    owner = Column(String, nullable=False)
    version = Column(String, nullable=False)
    character_set = Column(String, default="AL32UTF8")
    size_gb = Column(Integer, nullable=False)
    service_name = Column(String, nullable=True)
    pdb_name = Column(String, nullable=True)
    k8s_name = Column(String, nullable=True)
    k8s_namespace = Column(String, nullable=True)
    phase = Column(String, default="Pending")
    message = Column(Text, default="")
    created_at = Column(DateTime, default=lambda: datetime.now(timezone.utc))
    updated_at = Column(DateTime, default=lambda: datetime.now(timezone.utc))


Base.metadata.create_all(bind=engine)
SessionLocal = sessionmaker(bind=engine)

# ---------------------------------------------------------------------------
# SSE watch queue — broadcasts change events to all active Watch connections
# ---------------------------------------------------------------------------

_watch_subscribers: list[asyncio.Queue] = []


async def _simulate_provisioning(db_id: str) -> None:
    """Simulates async Oracle provisioning — moves phase from Creating/Starting → Ready after 8 seconds."""
    await asyncio.sleep(8)
    session = get_db()
    try:
        record = session.get(DatabaseRecord, db_id)
        if record and record.phase in ("Creating", "Starting"):
            record.phase = "Ready"
            record.message = "Database online and accepting connections"
            record.updated_at = datetime.now(timezone.utc)
            session.commit()
            session.refresh(record)
            _broadcast("MODIFIED", record)
    finally:
        session.close()


def _broadcast(event_type: str, record: DatabaseRecord) -> None:
    payload = json.dumps({"type": event_type, "object": _record_to_dict(record)})
    for q in _watch_subscribers:
        q.put_nowait(payload)


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _record_to_dict(r: DatabaseRecord) -> dict:
    return {
        "id": r.id,
        "dbName": r.db_name,
        "owner": r.owner,
        "version": r.version,
        "characterSet": r.character_set,
        "sizeGB": r.size_gb,
        "serviceName": r.service_name,
        "pdbName": r.pdb_name,
        "k8sName": r.k8s_name,
        "k8sNamespace": r.k8s_namespace,
        "phase": r.phase,
        "message": r.message,
        "createdAt": r.created_at.isoformat() if r.created_at else None,
        "updatedAt": r.updated_at.isoformat() if r.updated_at else None,
    }


def get_db() -> Session:
    return SessionLocal()


def _get_or_404(session: Session, db_id: str) -> DatabaseRecord:
    record = session.get(DatabaseRecord, db_id)
    if not record:
        raise HTTPException(status_code=404, detail=f"Database '{db_id}' not found")
    return record


# ---------------------------------------------------------------------------
# Pydantic schemas
# ---------------------------------------------------------------------------

class DatabaseCreate(BaseModel):
    dbName: str = Field(min_length=1, max_length=8)
    owner: str = Field(min_length=1)
    version: str = Field(min_length=1)
    characterSet: str = "AL32UTF8"
    sizeGB: int = Field(ge=1, le=65536)
    serviceName: Optional[str] = None
    pdbName: Optional[str] = None
    k8sName: Optional[str] = None
    k8sNamespace: Optional[str] = None


class DatabaseUpdate(BaseModel):
    dbName: Optional[str] = Field(default=None, min_length=1, max_length=8)
    owner: Optional[str] = Field(default=None, min_length=1)
    version: Optional[str] = Field(default=None, min_length=1)
    characterSet: Optional[str] = None
    sizeGB: Optional[int] = Field(default=None, ge=1, le=65536)
    serviceName: Optional[str] = None
    pdbName: Optional[str] = None


class StatusUpdate(BaseModel):
    phase: str
    message: Optional[str] = ""


# ---------------------------------------------------------------------------
# App
# ---------------------------------------------------------------------------

app = FastAPI(
    title="Mock Oracle Middleware API",
    description="Simulates an Oracle database provisioning middleware. Backed by SQLite.",
    version="1.0.0",
)


# ---------------------------------------------------------------------------
# Routes — order matters: /watch, /ui and /status must come before /{db_id}
# ---------------------------------------------------------------------------

@app.get("/ui", response_class=HTMLResponse, include_in_schema=False)
def ui():
    """Serve the web dashboard."""
    return (Path(__file__).parent / "static" / "index.html").read_text()


@app.get("/databases/watch")
async def watch_databases(request: Request):
    """Stream change events (ADDED, MODIFIED, DELETED) as Server-Sent Events."""
    queue: asyncio.Queue = asyncio.Queue()
    _watch_subscribers.append(queue)

    async def event_generator() -> AsyncGenerator[dict, None]:
        try:
            while True:
                if await request.is_disconnected():
                    break
                try:
                    payload = await asyncio.wait_for(queue.get(), timeout=15.0)
                    yield {"data": payload}
                except asyncio.TimeoutError:
                    yield {"comment": "keepalive"}
        finally:
            _watch_subscribers.remove(queue)

    return EventSourceResponse(event_generator())


@app.post("/databases", status_code=201)
def create_database(body: DatabaseCreate, background_tasks: BackgroundTasks):
    """Create a new database record. Phase starts as 'Creating', moves to 'Ready' after ~8s."""
    session = get_db()
    try:
        record = DatabaseRecord(
            db_name=body.dbName,
            owner=body.owner,
            version=body.version,
            character_set=body.characterSet,
            size_gb=body.sizeGB,
            service_name=body.serviceName,
            pdb_name=body.pdbName,
            k8s_name=body.k8sName,
            k8s_namespace=body.k8sNamespace,
            phase="Creating",
            message="Provisioning started",
        )
        session.add(record)
        session.commit()
        session.refresh(record)
        _broadcast("ADDED", record)
        background_tasks.add_task(_simulate_provisioning, record.id)
        return _record_to_dict(record)
    finally:
        session.close()


@app.get("/databases")
def list_databases():
    """Return all database records."""
    session = get_db()
    try:
        records = session.query(DatabaseRecord).all()
        return [_record_to_dict(r) for r in records]
    finally:
        session.close()


@app.get("/databases/{db_id}")
def get_database(db_id: str):
    """Return a single database record by ID."""
    session = get_db()
    try:
        return _record_to_dict(_get_or_404(session, db_id))
    finally:
        session.close()


@app.put("/databases/{db_id}")
def update_database(db_id: str, body: DatabaseCreate):
    """Full update — replaces all spec fields."""
    session = get_db()
    try:
        record = _get_or_404(session, db_id)
        record.db_name = body.dbName
        record.owner = body.owner
        record.version = body.version
        record.character_set = body.characterSet
        record.size_gb = body.sizeGB
        record.service_name = body.serviceName
        record.pdb_name = body.pdbName
        if body.k8sName is not None:
            record.k8s_name = body.k8sName
        if body.k8sNamespace is not None:
            record.k8s_namespace = body.k8sNamespace
        record.updated_at = datetime.now(timezone.utc)
        session.commit()
        session.refresh(record)
        _broadcast("MODIFIED", record)
        return _record_to_dict(record)
    finally:
        session.close()


@app.patch("/databases/{db_id}")
def patch_database(db_id: str, body: DatabaseUpdate):
    """Partial update — only provided fields are changed."""
    session = get_db()
    try:
        record = _get_or_404(session, db_id)
        if body.dbName is not None:
            record.db_name = body.dbName
        if body.owner is not None:
            record.owner = body.owner
        if body.version is not None:
            record.version = body.version
        if body.characterSet is not None:
            record.character_set = body.characterSet
        if body.sizeGB is not None:
            record.size_gb = body.sizeGB
        if body.serviceName is not None:
            record.service_name = body.serviceName
        if body.pdbName is not None:
            record.pdb_name = body.pdbName
        record.updated_at = datetime.now(timezone.utc)
        session.commit()
        session.refresh(record)
        _broadcast("MODIFIED", record)
        return _record_to_dict(record)
    finally:
        session.close()


@app.delete("/databases/{db_id}", status_code=204)
def delete_database(db_id: str):
    """Delete a database record."""
    session = get_db()
    try:
        record = _get_or_404(session, db_id)
        _broadcast("DELETED", record)
        session.delete(record)
        session.commit()
        return Response(status_code=204)
    finally:
        session.close()


@app.get("/databases/{db_id}/status")
def get_status(db_id: str):
    """Return only the status fields of a database record."""
    session = get_db()
    try:
        record = _get_or_404(session, db_id)
        return {"id": record.id, "phase": record.phase, "message": record.message}
    finally:
        session.close()


@app.put("/databases/{db_id}/status")
def update_status(db_id: str, body: StatusUpdate, background_tasks: BackgroundTasks):
    """Update only the status fields (phase and message).
    Setting phase to 'Creating' automatically triggers the provisioning simulation."""
    session = get_db()
    try:
        record = _get_or_404(session, db_id)
        record.phase = body.phase
        record.message = body.message or ""
        record.updated_at = datetime.now(timezone.utc)
        session.commit()
        session.refresh(record)
        _broadcast("MODIFIED", record)
        if body.phase in ("Creating", "Starting"):
            background_tasks.add_task(_simulate_provisioning, db_id)
        return {"id": record.id, "phase": record.phase, "message": record.message}
    finally:
        session.close()
