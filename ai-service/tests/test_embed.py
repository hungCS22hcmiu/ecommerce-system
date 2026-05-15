import pytest
from fastapi.testclient import TestClient
from main import app


@pytest.fixture(scope="session")
def client():
    with TestClient(app) as c:
        yield c


def test_health_live(client):
    r = client.get("/health/live")
    assert r.status_code == 200


def test_health_ready(client):
    r = client.get("/health/ready")
    assert r.status_code == 200
    assert r.json()["status"] == "ok"


def test_embed_returns_384d_vector(client):
    r = client.post("/embed", json={"text": "comfortable running shoes"})
    assert r.status_code == 200
    vec = r.json()["embedding"]
    assert len(vec) == 384
    assert all(isinstance(v, float) for v in vec)


def test_embed_batch(client):
    r = client.post("/embed/batch", json={"texts": ["shoes", "laptop", "book"]})
    assert r.status_code == 200
    vecs = r.json()["embeddings"]
    assert len(vecs) == 3
    assert all(len(v) == 384 for v in vecs)


def test_embed_batch_limit_enforced(client):
    r = client.post("/embed/batch", json={"texts": ["x"] * 65})
    assert r.status_code == 422


def test_similar_texts_closer_than_dissimilar(client):
    """Cosine similarity sanity: shoe-related texts rank closer to each other than to laptops."""
    def dot(a, b):
        return sum(x * y for x, y in zip(a, b))

    r1 = client.post("/embed", json={"text": "running shoes for long walks"}).json()["embedding"]
    r2 = client.post("/embed", json={"text": "comfortable footwear for hiking"}).json()["embedding"]
    r3 = client.post("/embed", json={"text": "laptop with fast processor"}).json()["embedding"]

    sim_shoes = dot(r1, r2)
    sim_cross = dot(r1, r3)
    assert sim_shoes > sim_cross, "semantic similarity check failed"
