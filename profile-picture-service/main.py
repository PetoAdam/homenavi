from fastapi import FastAPI, HTTPException, UploadFile, File, Form
from fastapi.responses import FileResponse
from PIL import Image, ImageDraw
import io
import os
import hashlib
import random
from typing import Optional
import uuid

app = FastAPI(title="Profile Picture Generator", version="1.0.0")

# Ensure uploads directory exists
UPLOAD_DIR = "/uploads"
os.makedirs(UPLOAD_DIR, exist_ok=True)

def generate_pixel_avatar(seed: str, size: int = 256) -> Image.Image:
    """Generate a pixel art avatar based on a seed string"""
    # Use seed to create deterministic random state
    random.seed(hashlib.md5(seed.encode()).hexdigest())
    
    # Create base image
    img = Image.new('RGB', (size, size), (240, 240, 240))
    draw = ImageDraw.Draw(img)
    
    # Grid size (8x8 pixels, each pixel will be size/8)
    grid_size = 8
    pixel_size = size // grid_size
    
    # Generate colors
    primary_color = (
        random.randint(50, 200),
        random.randint(50, 200), 
        random.randint(50, 200)
    )
    secondary_color = (
        min(255, primary_color[0] + 50),
        min(255, primary_color[1] + 50),
        min(255, primary_color[2] + 50)
    )
    
    # Generate symmetric pattern (only generate left half, mirror to right)
    for y in range(grid_size):
        for x in range(grid_size // 2):
            if random.random() > 0.5:  # 50% chance to fill pixel
                color = primary_color if random.random() > 0.3 else secondary_color
                
                # Draw left side
                left_x = x * pixel_size
                left_y = y * pixel_size
                draw.rectangle([
                    left_x, left_y,
                    left_x + pixel_size, left_y + pixel_size
                ], fill=color)
                
                # Mirror to right side
                right_x = (grid_size - 1 - x) * pixel_size
                draw.rectangle([
                    right_x, left_y,
                    right_x + pixel_size, left_y + pixel_size
                ], fill=color)
    
    return img

@app.get("/")
async def root():
    return {"message": "Profile Picture Generator Service", "version": "1.0.0"}

@app.get("/generate/{user_id}")
async def generate_avatar(user_id: str, size: int = 256):
    """Generate a pixel art avatar for a user"""
    try:
        # Generate deterministic avatar based on user_id
        avatar = generate_pixel_avatar(user_id, size)
        
        # Save to uploads directory
        filename = f"avatar_{user_id}_{size}.png"
        filepath = os.path.join(UPLOAD_DIR, filename)
        avatar.save(filepath, "PNG")
        
        return {
            "success": True,
            "filename": filename,
            "url": f"/uploads/{filename}",
            "size": size
        }
    except Exception as e:
        raise HTTPException(status_code=500, detail=f"Avatar generation failed: {str(e)}")

@app.post("/upload")
async def upload_profile_picture(
    user_id: str = Form(...),
    file: UploadFile = File(...)
):
    """Upload a custom profile picture"""
    try:
        # Validate file type
        if not file.content_type or not file.content_type.startswith('image/'):
            raise HTTPException(status_code=400, detail="File must be an image")
        
        # Validate file size (max 5MB)
        content = await file.read()
        if len(content) > 5 * 1024 * 1024:
            raise HTTPException(status_code=400, detail="File too large (max 5MB)")
        
        # Process image
        image = Image.open(io.BytesIO(content))
        
        # Convert to RGB if necessary
        if image.mode != 'RGB':
            image = image.convert('RGB')
        
        # Resize to standard sizes
        sizes = [64, 128, 256]
        filenames = []
        
        for size in sizes:
            # Resize maintaining aspect ratio
            image_resized = image.copy()
            image_resized.thumbnail((size, size), Image.Resampling.LANCZOS)
            
            # Create square image with padding if needed
            square_img = Image.new('RGB', (size, size), (255, 255, 255))
            offset = ((size - image_resized.width) // 2, (size - image_resized.height) // 2)
            square_img.paste(image_resized, offset)
            
            # Save file
            filename = f"profile_{user_id}_{size}_{uuid.uuid4().hex[:8]}.png"
            filepath = os.path.join(UPLOAD_DIR, filename)
            square_img.save(filepath, "PNG")
            filenames.append(filename)
        
        return {
            "success": True,
            "filenames": filenames,
            "urls": [f"/uploads/{fn}" for fn in filenames],
            "primary_url": f"/uploads/{filenames[2]}"  # 256px version
        }
        
    except HTTPException:
        raise
    except Exception as e:
        print(f"Upload error: {str(e)}")  # Add logging
        raise HTTPException(status_code=500, detail=f"Upload failed: {str(e)}")

@app.get("/health")
async def health_check():
    return {"status": "healthy"}