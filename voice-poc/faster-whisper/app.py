import os
import tempfile

from fastapi import FastAPI, File, HTTPException, UploadFile
from faster_whisper import WhisperModel

app = FastAPI()
model = WhisperModel(
    os.getenv("WHISPER_MODEL", "base.en"),
    device=os.getenv("WHISPER_DEVICE", "cuda"),
    compute_type=os.getenv("WHISPER_COMPUTE_TYPE", "float16"),
)


@app.get("/healthz")
def healthz():
    return {"status": "ok"}


@app.post("/v1/audio/transcriptions")
async def transcribe(file: UploadFile = File(...)):
    if file.content_type not in {"audio/wav", "application/octet-stream"}:
        raise HTTPException(status_code=415, detail="expected WAV audio")
    with tempfile.NamedTemporaryFile(suffix=".wav") as temporary:
        temporary.write(await file.read())
        temporary.flush()
        segments, _ = model.transcribe(
            temporary.name,
            beam_size=5,
            condition_on_previous_text=False,
            vad_filter=True,
        )
        text = "".join(segment.text for segment in segments).strip()
    return {"text": text}