from contextlib import asynccontextmanager
from typing import Optional
import asyncio
import os

from fastapi import FastAPI, HTTPException
from pydantic import BaseModel, Field
from sentence_transformers import SentenceTransformer

MODEL_NAME = os.getenv("EMBEDDING_MODEL", "all-MiniLM-L6-v2")
_model: Optional[SentenceTransformer] = None


@asynccontextmanager
async def lifespan(app: FastAPI):
    global _model
    loop = asyncio.get_event_loop()
    _model = await loop.run_in_executor(None, SentenceTransformer, MODEL_NAME)
    yield
    _model = None


app = FastAPI(lifespan=lifespan)


class EmbedRequest(BaseModel):
    text: str


class EmbedResponse(BaseModel):
    embedding: list[float]


class BatchEmbedRequest(BaseModel):
    texts: list[str] = Field(..., max_length=64)


class BatchEmbedResponse(BaseModel):
    embeddings: list[list[float]]


@app.get("/health/live")
def health_live():
    return {"status": "ok"}


@app.get("/health/ready")
def health_ready():
    if _model is None:
        raise HTTPException(status_code=503, detail="model not loaded")
    return {"status": "ok", "model": MODEL_NAME}


@app.post("/embed", response_model=EmbedResponse)
def embed(req: EmbedRequest):
    if _model is None:
        raise HTTPException(status_code=503, detail="model not loaded")
    vec = _model.encode(req.text, normalize_embeddings=True).tolist()
    return EmbedResponse(embedding=vec)


@app.post("/embed/batch", response_model=BatchEmbedResponse)
def embed_batch(req: BatchEmbedRequest):
    if _model is None:
        raise HTTPException(status_code=503, detail="model not loaded")
    if len(req.texts) > 64:
        raise HTTPException(status_code=422, detail="max 64 texts per batch")
    vecs = _model.encode(req.texts, normalize_embeddings=True).tolist()
    return BatchEmbedResponse(embeddings=vecs)
