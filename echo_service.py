import asyncio
import websockets

async def echo(websocket):
    async for message in websocket:
        print(f"Received message: {message}")
        await websocket.send(f"echo: {message}")

if __name__ == "__main__":
    import os
    port = int(os.environ.get("ECHO_PORT", 9000))
    async def main():
        async with websockets.serve(echo, "0.0.0.0", port):
            print(f"Echo WebSocket server running on port {port}")
            await asyncio.Future()  # run forever
    asyncio.run(main())
