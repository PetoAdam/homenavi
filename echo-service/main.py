#!/usr/bin/env python3
"""
Simple WebSocket Echo Service for testing JWT authentication
"""
import asyncio
import websockets
import json
import logging
from datetime import datetime

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

async def echo_handler(websocket, path):
    """Handle WebSocket connections and echo back messages"""
    client_ip = websocket.remote_address[0]
    logger.info(f"New WebSocket connection from {client_ip} on path {path}")
    
    try:
        # Send welcome message
        welcome_msg = {
            "type": "welcome",
            "message": "WebSocket Echo Service - JWT Auth Test",
            "timestamp": datetime.now().isoformat(),
            "path": path
        }
        await websocket.send(json.dumps(welcome_msg))
        
        # Echo loop
        async for message in websocket:
            try:
                logger.info(f"Received from {client_ip}: {message}")
                
                # Try to parse as JSON, otherwise treat as plain text
                try:
                    data = json.loads(message)
                    response = {
                        "type": "echo",
                        "original": data,
                        "timestamp": datetime.now().isoformat(),
                        "client_ip": client_ip
                    }
                except json.JSONDecodeError:
                    response = {
                        "type": "echo",
                        "original": message,
                        "timestamp": datetime.now().isoformat(),
                        "client_ip": client_ip
                    }
                
                await websocket.send(json.dumps(response))
                
            except Exception as e:
                logger.error(f"Error processing message: {e}")
                error_response = {
                    "type": "error",
                    "error": str(e),
                    "timestamp": datetime.now().isoformat()
                }
                await websocket.send(json.dumps(error_response))
                
    except websockets.exceptions.ConnectionClosed:
        logger.info(f"Connection closed for {client_ip}")
    except Exception as e:
        logger.error(f"Error in echo_handler: {e}")

async def main():
    """Start the WebSocket server"""
    host = "0.0.0.0"
    port = 9000
    
    logger.info(f"Starting WebSocket Echo Service on {host}:{port}")
    
    async with websockets.serve(echo_handler, host, port):
        logger.info("WebSocket Echo Service is running...")
        await asyncio.Future()  # Run forever

if __name__ == "__main__":
    asyncio.run(main())
