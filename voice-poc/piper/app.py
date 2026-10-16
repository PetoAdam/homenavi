import os
import urllib.request

from fastapi import FastAPI, HTTPException
from fastapi.responses import StreamingResponse
from piper import PiperVoice
from pydantic import BaseModel

MODEL_DIR = "/models"
MODEL_NAME = os.getenv("PIPER_VOICE", "en_US-lessac-medium")
MODEL_URL = os.getenv(
    "PIPER_MODEL_URL",
    "https://huggingface.co/rhasspy/piper-voices/resolve/main/en/en_US/lessac/medium/en_US-lessac-medium.onnx",
)


def ensure_voice():
    os.makedirs(MODEL_DIR, exist_ok=True)
    model_path = os.path.join(MODEL_DIR, f"{MODEL_NAME}.onnx")
    config_path = f"{model_path}.json"
    if not os.path.exists(model_path):
        urllib.request.urlretrieve(MODEL_URL, model_path)
    if not os.path.exists(config_path):
        urllib.request.urlretrieve(f"{MODEL_URL}.json", config_path)
    return PiperVoice.load(model_path)


voice = ensure_voice()
app = FastAPI()


class Request(BaseModel):
    text: str


@app.get("/healthz")
def healthz():
    return {"status": "ok", "voice": MODEL_NAME}


@app.post("/tts")
def tts(request: Request):
    text = request.text.strip()
    if not text or len(text) > 1000:
        raise HTTPException(status_code=400, detail="text must be 1-1000 characters")

    def pcm_chunks():
        for chunk in voice.synthesize(text):
            yield chunk.audio_int16_bytes

    return StreamingResponse(
        pcm_chunks(),
        media_type="audio/L16",
        headers={
            "X-Audio-Sample-Rate": str(voice.config.sample_rate),
            "X-Audio-Sample-Width": "2",
            "X-Audio-Channels": "1",
        },
    )