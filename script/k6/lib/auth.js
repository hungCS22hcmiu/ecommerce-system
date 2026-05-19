// Shared helpers for Phase 2 k6 scripts.
import http from 'k6/http';
import { uuidv4 } from 'https://jslib.k6.io/k6-utils/1.4.0/index.js';

export function login(authUrl, email, password) {
  const res = http.post(
    `${authUrl}/api/v1/auth/login`,
    JSON.stringify({ email, password }),
    { headers: { 'Content-Type': 'application/json' } },
  );
  if (res.status !== 200) {
    throw new Error(`Login ${email}: ${res.status} ${res.body}`);
  }
  const body = res.json();
  return { token: body.data.access_token, userId: body.data.user.id };
}

export function seedHighStockProduct(productUrl, sellerUserId, stockQuantity, namePrefix) {
  const payload = JSON.stringify({
    name: `${namePrefix || 'phase2-test'}-${uuidv4()}`,
    description: 'Phase 2 auto-seeded test product',
    price: 9.99,
    categoryId: 1,
    stockQuantity,
  });
  const res = http.post(`${productUrl}/api/v1/products`, payload, {
    headers: { 'Content-Type': 'application/json', 'X-Seller-Id': sellerUserId },
  });
  if (res.status !== 201) {
    throw new Error(`seed product failed: ${res.status} ${res.body}`);
  }
  return res.json('data.id');
}
