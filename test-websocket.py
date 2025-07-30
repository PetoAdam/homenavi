#!/usr/bin/env python3
"""
WebSocket JWT Authentication Test Script
This script establishes real WebSocket connections and tests JWT authentication
exactly like a browser would.
"""

import asyncio
import websockets
import json
import sys
import time
from datetime import datetime
import urllib.request
import urllib.error

# Colors for terminal output
class Colors:
    RED = '\033[0;31m'
    GREEN = '\033[0;32m'
    YELLOW = '\033[1;33m'
    BLUE = '\033[0;34m'
    PURPLE = '\033[0;35m'
    CYAN = '\033[0;36m'
    NC = '\033[0m'  # No Color

def print_colored(text, color):
    print(f"{color}{text}{Colors.NC}")

def print_header(text):
    print_colored(f"\n=== {text} ===", Colors.BLUE)

def print_success(text):
    print_colored(f"✓ {text}", Colors.GREEN)

def print_error(text):
    print_colored(f"✗ {text}", Colors.RED)

def print_warning(text):
    print_colored(f"⚠ {text}", Colors.YELLOW)

def print_info(text):
    print_colored(f"ℹ {text}", Colors.CYAN)

class WebSocketTester:
    def __init__(self):
        self.test_results = []
        
    async def test_websocket_connection(self, uri, description, headers=None, cookies=None, expect_success=True):
        """Test a WebSocket connection with proper browser-like behavior"""
        print_header(f"Test: {description}")
        print_info(f"Connecting to: {uri}")
        
        # Prepare extra headers (like a browser would send)
        extra_headers = {
            "User-Agent": "Mozilla/5.0 (WebSocket Test Client)",
            "Origin": "http://localhost"
        }
        
        if headers:
            extra_headers.update(headers)
            
        if cookies:
            # Format cookies like a browser would
            cookie_string = "; ".join([f"{k}={v}" for k, v in cookies.items()])
            extra_headers["Cookie"] = cookie_string
            print_info(f"Sending cookies: {cookie_string}")
        
        if "Authorization" in extra_headers:
            print_info(f"Sending Authorization header")
            
        print_info("Headers being sent:")
        for key, value in extra_headers.items():
            if key == "Authorization":
                print(f"  {key}: Bearer <token>")
            else:
                print(f"  {key}: {value}")
        
        try:
            # Connect with timeout
            print_info("Attempting WebSocket connection...")
            
            async with websockets.connect(
                uri,
                extra_headers=extra_headers,
                ping_interval=None,  # Disable ping for testing
                close_timeout=5,
                open_timeout=10
            ) as websocket:
                print_success("WebSocket connection established!")
                
                # Test echo functionality
                await self.test_echo_functionality(websocket)
                
                if expect_success:
                    self.test_results.append((description, "PASS", "Connection successful"))
                    return True
                else:
                    self.test_results.append((description, "UNEXPECTED", "Expected failure but succeeded"))
                    print_warning("Expected this connection to fail, but it succeeded")
                    return False
                    
        except websockets.exceptions.InvalidStatusCode as e:
            error_msg = f"HTTP {e.status_code}"
            if e.status_code == 401:
                print_error(f"Authentication failed: {error_msg}")
                if not expect_success:
                    self.test_results.append((description, "PASS", f"Expected failure: {error_msg}"))
                    return True
                else:
                    self.test_results.append((description, "FAIL", f"Authentication failed: {error_msg}"))
                    return False
            elif e.status_code == 403:
                print_error(f"Authorization failed: {error_msg}")
                self.test_results.append((description, "FAIL", f"Authorization failed: {error_msg}"))
                return False
            elif e.status_code == 502:
                print_error(f"Bad Gateway: {error_msg} - Service might be down")
                self.test_results.append((description, "FAIL", f"Service unavailable: {error_msg}"))
                return False
            else:
                print_error(f"HTTP error: {error_msg}")
                self.test_results.append((description, "FAIL", f"HTTP error: {error_msg}"))
                return False
                
        except websockets.exceptions.ConnectionClosed as e:
            print_error(f"Connection closed: {e}")
            self.test_results.append((description, "FAIL", f"Connection closed: {e}"))
            return False
            
        except asyncio.TimeoutError:
            print_error("Connection timeout")
            self.test_results.append((description, "FAIL", "Connection timeout"))
            return False
            
        except Exception as e:
            print_error(f"Connection failed: {str(e)}")
            self.test_results.append((description, "FAIL", f"Connection error: {str(e)}"))
            return False

    async def test_echo_functionality(self, websocket):
        """Test the echo functionality like a browser would"""
        print_info("Testing echo functionality...")
        
        try:
            # Wait for welcome message
            welcome_msg = await asyncio.wait_for(websocket.recv(), timeout=5)
            print_success(f"Received welcome: {welcome_msg}")
            
            # Test 1: Send plain text
            test_message = "Hello from WebSocket test!"
            print_info(f"Sending: {test_message}")
            await websocket.send(test_message)
            
            response = await asyncio.wait_for(websocket.recv(), timeout=5)
            print_success(f"Echo response: {response}")
            
            # Test 2: Send JSON
            test_json = {
                "type": "test",
                "message": "Hello JSON!",
                "timestamp": datetime.now().isoformat()
            }
            json_message = json.dumps(test_json)
            print_info(f"Sending JSON: {json_message}")
            await websocket.send(json_message)
            
            json_response = await asyncio.wait_for(websocket.recv(), timeout=5)
            print_success(f"JSON echo response: {json_response}")
            
            # Verify we got proper echo responses
            try:
                parsed_response = json.loads(json_response)
                if parsed_response.get("type") == "echo":
                    print_success("Echo functionality working correctly!")
                else:
                    print_warning("Unexpected response format")
            except json.JSONDecodeError:
                print_warning("Response was not valid JSON")
                
        except asyncio.TimeoutError:
            print_error("Timeout waiting for echo response")
        except Exception as e:
            print_error(f"Echo test failed: {e}")

    async def check_services(self):
        """Check if required services are running"""
        print_header("Checking Services")
        
        # Only check nginx since that's what we're testing
        services = [
            ("http://localhost", "Nginx"),
        ]
        
        for url, name in services:
            try:
                # Use urllib instead of aiohttp to avoid external dependency
                request = urllib.request.Request(url)
                with urllib.request.urlopen(request, timeout=5) as response:
                    print_success(f"{name} is running ({url})")
            except Exception as e:
                print_error(f"{name} is not responding ({url}): {e}")
                return False
                
        print_info("Note: WebSocket services will show 'Upgrade Required' on HTTP requests - this is normal")
        print_success("All required services are accessible")
        return True

    def get_jwt_token(self):
        """Get JWT token from user input"""
        print_header("JWT Token Input")
        print("Please enter your JWT token:")
        print("(You can get this from your browser's developer tools after logging in)")
        print("- Chrome/Firefox: F12 > Application/Storage > Cookies > auth_token")
        print("- Or from localStorage > token")
        print()
        
        token = input("JWT Token: ").strip()
        if not token:
            print_warning("No token provided")
            return None
            
        # Validate token format (basic check)
        if not token.startswith(('eyJ', 'ey')):
            print_warning("Token doesn't look like a JWT (should start with 'eyJ' or 'ey')")
            
        return token

    async def run_all_tests(self):
        """Run all WebSocket authentication tests"""
        print_colored("🚀 WebSocket JWT Authentication Test Suite", Colors.PURPLE)
        print_colored("=" * 50, Colors.PURPLE)
        
        # Check if nginx is running
        if not await self.check_services():
            print_error("Nginx is not running. Please start docker-compose.")
            return False
        
        # Test 1: No authentication (should fail)
        await self.test_websocket_connection(
            "ws://localhost/ws/echo",
            "No Authentication (via Nginx)",
            expect_success=False
        )
        
        # Get JWT token
        jwt_token = self.get_jwt_token()
        
        if jwt_token:
            # Test 2: Cookie authentication (primary method)
            await self.test_websocket_connection(
                "ws://localhost/ws/echo",
                "Cookie Authentication (via Nginx)",
                cookies={"auth_token": jwt_token},
                expect_success=True
            )
            
            # Test 3: Authorization header (fallback method)
            await self.test_websocket_connection(
                "ws://localhost/ws/echo",
                "Authorization Header (via Nginx)",
                headers={"Authorization": f"Bearer {jwt_token}"},
                expect_success=True
            )
        else:
            print_warning("Skipping authenticated tests - no token provided")
        
        # Print test summary
        self.print_test_summary()
        
        return True

    def print_test_summary(self):
        """Print a summary of all test results"""
        print_header("Test Summary")
        
        for description, result, details in self.test_results:
            if result == "PASS":
                print_success(f"{description}: {result}")
            elif result == "FAIL":
                print_error(f"{description}: {result} - {details}")
            else:
                print_warning(f"{description}: {result} - {details}")
        
        passed = sum(1 for _, result, _ in self.test_results if result == "PASS")
        total = len(self.test_results)
        
        print_colored(f"\nResults: {passed}/{total} tests passed", Colors.PURPLE)
        
        if passed == total:
            print_success("🎉 All tests passed! Your WebSocket JWT authentication is working correctly.")
        elif passed > 0:
            print_warning("⚠️  Some tests failed. Check the details above.")
        else:
            print_error("❌ All tests failed. Check your configuration and services.")

async def main():
    """Main entry point"""
    try:
        tester = WebSocketTester()
        await tester.run_all_tests()
    except KeyboardInterrupt:
        print_colored("\n\n🛑 Test interrupted by user", Colors.YELLOW)
    except Exception as e:
        print_error(f"Unexpected error: {e}")
        sys.exit(1)

if __name__ == "__main__":
    # Check if required packages are available
    try:
        import websockets
    except ImportError as e:
        print_error(f"Missing required package: {e}")
        print_info("Install with: pip install websockets")
        sys.exit(1)
    
    # Run the async main function
    asyncio.run(main())
