// Simple test login service for development/demo

export async function loginTestUser(email, password) {
  // Simulate async delay
  await new Promise(res => setTimeout(res, 300));
  if (email === 'test@test.com' && password === 'test') {
    return {
      success: true,
      name: 'Test User',
      email: 'test@test.com',
      avatar: 'https://randomuser.me/api/portraits/men/32.jpg',
    };
  }
  return { success: false };
}
