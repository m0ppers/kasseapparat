import * as Sentry from "@sentry/react";
import Decimal from "decimal.js";

// Unified error handler for failed fetch responses
const handleFetchError = async (response) => {
  let message = `HTTP ${response.status} ${response.statusText}`;
  try {
    const data = await response.json();
    message = data?.details || data?.error || data?.message || message;
  } catch {
    // Ignore invalid JSON
  }
  const error = new Error(message);
  Sentry.captureException(error, {
    extra: {
      url: response.url,
      status: response.status,
    },
  });
  throw error;
};

// Authenticated GET helper
const get = async (url) => {
  const response = await fetch(url, {
    method: "GET",
    headers: {
      "Content-Type": "application/json",
    },
  });
  if (!response.ok) await handleFetchError(response);
  return response.json();
};

// Authenticated POST helper
const post = async (url, body) => {
  const response = await fetch(url, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(body),
  });
  if (!response.ok) await handleFetchError(response);
  return response.json();
};

// Fetch all visible products
export const fetchProducts = async (apiHost) => {
  const url = `${apiHost}/api/v2/products?_end=1000&_sort=pos&_order=asc&_filter_hidden=true`;
  return get(url);
};

// Fetch guests for a specific product
export const fetchGuestlistByProductId = async (apiHost, productId, query) => {
  const url = `${apiHost}/api/v2/products/${productId}/guests?q=${query}`;
  return get(url);
};

// Store a new purchase
export const storePurchase = async (
  apiHost,
  cart,
  paymentMethodCode,
  paymentMethodData = {},
) => {
  const cartPayload = cart.map((item) => ({
    ...item,
    lists: null,
    guestlists: null,
  }));

  const payload = {
    paymentMethod: paymentMethodCode,
    cart: cartPayload,
    totalGrossPrice: cart.reduce(
      (total, item) => total.add(item.totalGrossPrice),
      new Decimal(0),
    ),
    totalNetPrice: cart.reduce(
      (total, item) => total.add(item.totalNetPrice),
      new Decimal(0),
    ),
    ...paymentMethodData,
  };

  return post(`${apiHost}/api/v2/purchases`, payload);
};

// Fetch all confirmed purchases for a user
export const fetchPurchases = async (apiHost, userId) => {
  const url = `${apiHost}/api/v2/purchases?createdById=${encodeURIComponent(userId)}&status=confirmed`;
  return get(url);
};

// Refund a purchase by ID
export const refundPurchaseById = async (apiHost, purchaseId) => {
  const url = `${apiHost}/api/v2/purchases/${purchaseId}/refund`;
  return post(url, {});
};

// Add interest in a product
export const addProductInterest = async (apiHost, productId) => {
  return post(`${apiHost}/api/v2/productInterests`, { productId });
};
