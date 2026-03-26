// Package templates ecommerceTemplate provides an ecommerce domain scaffold with production-style schema, seed data, typed client SDK code, and documentation.
package templates

type ecommerceTemplate struct{}

func init() {
	Register(ecommerceTemplate{})
}

func (ecommerceTemplate) Name() string {
	return "ecommerce"
}

// Schema returns the SQL DDL for the ecommerce domain, creating tables for products, customers, orders, order items, and carts with row-level security policies enforcing user isolation.
func (ecommerceTemplate) Schema() string {
	return `-- Ecommerce domain schema
-- Apply with: ayb sql < schema.sql

CREATE TABLE IF NOT EXISTS products (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    description TEXT NOT NULL,
    price_cents INTEGER NOT NULL CHECK (price_cents >= 0),
    currency TEXT NOT NULL DEFAULT 'USD',
    sku TEXT UNIQUE,
    stock_count INTEGER NOT NULL DEFAULT 0 CHECK (stock_count >= 0),
    image_url TEXT,
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS customers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL UNIQUE REFERENCES _ayb_users(id),
    name TEXT NOT NULL,
    email TEXT NOT NULL,
    shipping_address JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS orders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id UUID NOT NULL REFERENCES customers(id),
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'paid', 'shipped', 'delivered', 'cancelled')),
    total_cents INTEGER NOT NULL DEFAULT 0 CHECK (total_cents >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS order_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    product_id UUID NOT NULL REFERENCES products(id),
    quantity INTEGER NOT NULL CHECK (quantity > 0),
    unit_price_cents INTEGER NOT NULL CHECK (unit_price_cents >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS carts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL UNIQUE REFERENCES _ayb_users(id),
    items JSONB NOT NULL DEFAULT '[]',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE products ENABLE ROW LEVEL SECURITY;
ALTER TABLE customers ENABLE ROW LEVEL SECURITY;
ALTER TABLE orders ENABLE ROW LEVEL SECURITY;
ALTER TABLE order_items ENABLE ROW LEVEL SECURITY;
ALTER TABLE carts ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS products_select ON products;
CREATE POLICY products_select ON products FOR SELECT
    USING (active = true);

DROP POLICY IF EXISTS products_insert ON products;
CREATE POLICY products_insert ON products FOR INSERT
    WITH CHECK (current_setting('ayb.user_id', true)::uuid IS NOT NULL);

DROP POLICY IF EXISTS products_update ON products;
CREATE POLICY products_update ON products FOR UPDATE
    USING (current_setting('ayb.user_id', true)::uuid IS NOT NULL)
    WITH CHECK (current_setting('ayb.user_id', true)::uuid IS NOT NULL);

DROP POLICY IF EXISTS products_delete ON products;
CREATE POLICY products_delete ON products FOR DELETE
    USING (current_setting('ayb.user_id', true)::uuid IS NOT NULL);

DROP POLICY IF EXISTS customers_select ON customers;
CREATE POLICY customers_select ON customers FOR SELECT
    USING (user_id = current_setting('ayb.user_id', true)::uuid);

DROP POLICY IF EXISTS customers_insert ON customers;
CREATE POLICY customers_insert ON customers FOR INSERT
    WITH CHECK (user_id = current_setting('ayb.user_id', true)::uuid);

DROP POLICY IF EXISTS customers_update ON customers;
CREATE POLICY customers_update ON customers FOR UPDATE
    USING (user_id = current_setting('ayb.user_id', true)::uuid)
    WITH CHECK (user_id = current_setting('ayb.user_id', true)::uuid);

DROP POLICY IF EXISTS customers_delete ON customers;
CREATE POLICY customers_delete ON customers FOR DELETE
    USING (user_id = current_setting('ayb.user_id', true)::uuid);

DROP POLICY IF EXISTS orders_select ON orders;
CREATE POLICY orders_select ON orders FOR SELECT
    USING (
        EXISTS (
            SELECT 1
            FROM customers c
            WHERE c.id = orders.customer_id
              AND c.user_id = current_setting('ayb.user_id', true)::uuid
        )
    );

DROP POLICY IF EXISTS orders_insert ON orders;
CREATE POLICY orders_insert ON orders FOR INSERT
    WITH CHECK (
        EXISTS (
            SELECT 1
            FROM customers c
            WHERE c.id = orders.customer_id
              AND c.user_id = current_setting('ayb.user_id', true)::uuid
        )
    );

DROP POLICY IF EXISTS orders_update ON orders;
CREATE POLICY orders_update ON orders FOR UPDATE
    USING (
        EXISTS (
            SELECT 1
            FROM customers c
            WHERE c.id = orders.customer_id
              AND c.user_id = current_setting('ayb.user_id', true)::uuid
        )
    )
    WITH CHECK (
        EXISTS (
            SELECT 1
            FROM customers c
            WHERE c.id = orders.customer_id
              AND c.user_id = current_setting('ayb.user_id', true)::uuid
        )
    );

DROP POLICY IF EXISTS orders_delete ON orders;
CREATE POLICY orders_delete ON orders FOR DELETE
    USING (
        EXISTS (
            SELECT 1
            FROM customers c
            WHERE c.id = orders.customer_id
              AND c.user_id = current_setting('ayb.user_id', true)::uuid
        )
    );

DROP POLICY IF EXISTS order_items_select ON order_items;
CREATE POLICY order_items_select ON order_items FOR SELECT
    USING (
        EXISTS (
            SELECT 1
            FROM orders o
            JOIN customers c ON c.id = o.customer_id
            WHERE o.id = order_items.order_id
              AND c.user_id = current_setting('ayb.user_id', true)::uuid
        )
    );

DROP POLICY IF EXISTS order_items_insert ON order_items;
CREATE POLICY order_items_insert ON order_items FOR INSERT
    WITH CHECK (
        EXISTS (
            SELECT 1
            FROM orders o
            JOIN customers c ON c.id = o.customer_id
            WHERE o.id = order_items.order_id
              AND c.user_id = current_setting('ayb.user_id', true)::uuid
        )
    );

DROP POLICY IF EXISTS order_items_update ON order_items;
CREATE POLICY order_items_update ON order_items FOR UPDATE
    USING (
        EXISTS (
            SELECT 1
            FROM orders o
            JOIN customers c ON c.id = o.customer_id
            WHERE o.id = order_items.order_id
              AND c.user_id = current_setting('ayb.user_id', true)::uuid
        )
    )
    WITH CHECK (
        EXISTS (
            SELECT 1
            FROM orders o
            JOIN customers c ON c.id = o.customer_id
            WHERE o.id = order_items.order_id
              AND c.user_id = current_setting('ayb.user_id', true)::uuid
        )
    );

DROP POLICY IF EXISTS order_items_delete ON order_items;
CREATE POLICY order_items_delete ON order_items FOR DELETE
    USING (
        EXISTS (
            SELECT 1
            FROM orders o
            JOIN customers c ON c.id = o.customer_id
            WHERE o.id = order_items.order_id
              AND c.user_id = current_setting('ayb.user_id', true)::uuid
        )
    );

DROP POLICY IF EXISTS carts_select ON carts;
CREATE POLICY carts_select ON carts FOR SELECT
    USING (user_id = current_setting('ayb.user_id', true)::uuid);

DROP POLICY IF EXISTS carts_insert ON carts;
CREATE POLICY carts_insert ON carts FOR INSERT
    WITH CHECK (user_id = current_setting('ayb.user_id', true)::uuid);

DROP POLICY IF EXISTS carts_update ON carts;
CREATE POLICY carts_update ON carts FOR UPDATE
    USING (user_id = current_setting('ayb.user_id', true)::uuid)
    WITH CHECK (user_id = current_setting('ayb.user_id', true)::uuid);

DROP POLICY IF EXISTS carts_delete ON carts;
CREATE POLICY carts_delete ON carts FOR DELETE
    USING (user_id = current_setting('ayb.user_id', true)::uuid);
`
}

// SeedData returns SQL statements that populate the ecommerce schema with sample users, customers, products with varying inventory levels, orders in different lifecycle states, order items, and shopping carts.
func (ecommerceTemplate) SeedData() string {
	return `-- Ecommerce domain seed data
-- Apply with: ayb sql < seed.sql

INSERT INTO _ayb_users (id, email, password_hash)
VALUES
    ('51111111-1111-1111-1111-111111111111', 'shopper.one@example.com', 'seeded-password-hash'),
    ('52222222-2222-2222-2222-222222222222', 'shopper.two@example.com', 'seeded-password-hash'),
    ('53333333-3333-3333-3333-333333333333', 'shopper.three@example.com', 'seeded-password-hash')
ON CONFLICT DO NOTHING;

INSERT INTO customers (id, user_id, name, email, shipping_address)
VALUES
    ('61000000-0000-0000-0000-000000000001', '51111111-1111-1111-1111-111111111111', 'Jordan Miles', 'shopper.one@example.com', '{"line1":"100 Market St","city":"New York","state":"NY","postal_code":"10001","country":"US"}'::jsonb),
    ('61000000-0000-0000-0000-000000000002', '52222222-2222-2222-2222-222222222222', 'Avery Chen', 'shopper.two@example.com', '{"line1":"250 Mission St","city":"San Francisco","state":"CA","postal_code":"94105","country":"US"}'::jsonb),
    ('61000000-0000-0000-0000-000000000003', '53333333-3333-3333-3333-333333333333', 'Riley Patel', 'shopper.three@example.com', '{"line1":"77 Wacker Dr","city":"Chicago","state":"IL","postal_code":"60601","country":"US"}'::jsonb)
ON CONFLICT DO NOTHING;

INSERT INTO products (id, name, description, price_cents, currency, sku, stock_count, image_url, active)
VALUES
    ('62000000-0000-0000-0000-000000000001', 'Mechanical Keyboard', 'Hot-swappable mechanical keyboard with RGB.', 12999, 'USD', 'KB-001', 42, 'https://example.com/images/keyboard.jpg', true),
    ('62000000-0000-0000-0000-000000000002', 'Ergonomic Mouse', 'Vertical ergonomic mouse for all-day comfort.', 6999, 'USD', 'MS-002', 55, 'https://example.com/images/mouse.jpg', true),
    ('62000000-0000-0000-0000-000000000003', '4K Monitor', '27-inch 4K monitor with USB-C.', 32999, 'USD', 'MN-003', 18, 'https://example.com/images/monitor.jpg', true),
    ('62000000-0000-0000-0000-000000000004', 'Laptop Stand', 'Aluminum stand for 13-16 inch laptops.', 3999, 'USD', 'ST-004', 73, 'https://example.com/images/stand.jpg', true),
    ('62000000-0000-0000-0000-000000000005', 'USB-C Dock', '10-in-1 USB-C docking station.', 15999, 'USD', 'DK-005', 30, 'https://example.com/images/dock.jpg', true),
    ('62000000-0000-0000-0000-000000000006', 'Noise-Canceling Headphones', 'Wireless over-ear ANC headphones.', 24999, 'USD', 'HP-006', 12, 'https://example.com/images/headphones.jpg', true),
    ('62000000-0000-0000-0000-000000000007', 'Webcam', '1080p webcam with stereo microphones.', 8999, 'USD', 'WC-007', 28, 'https://example.com/images/webcam.jpg', true),
    ('62000000-0000-0000-0000-000000000008', 'Desk Mat', 'Large anti-slip desk mat.', 2999, 'USD', 'DM-008', 120, 'https://example.com/images/deskmat.jpg', true),
    ('62000000-0000-0000-0000-000000000009', 'Portable SSD', '1TB USB-C portable SSD.', 10999, 'USD', 'SD-009', 0, 'https://example.com/images/ssd.jpg', false),
    ('62000000-0000-0000-0000-000000000010', 'LED Light Bar', 'Adjustable monitor light bar.', 4999, 'USD', 'LB-010', 44, 'https://example.com/images/lightbar.jpg', false)
ON CONFLICT DO NOTHING;

INSERT INTO orders (id, customer_id, status, total_cents)
VALUES
    ('63000000-0000-0000-0000-000000000001', '61000000-0000-0000-0000-000000000001', 'pending', 22997),
    ('63000000-0000-0000-0000-000000000002', '61000000-0000-0000-0000-000000000001', 'paid', 37998),
    ('63000000-0000-0000-0000-000000000003', '61000000-0000-0000-0000-000000000002', 'shipped', 37997),
    ('63000000-0000-0000-0000-000000000004', '61000000-0000-0000-0000-000000000003', 'delivered', 15997),
    ('63000000-0000-0000-0000-000000000005', '61000000-0000-0000-0000-000000000002', 'cancelled', 15999)
ON CONFLICT DO NOTHING;

INSERT INTO order_items (id, order_id, product_id, quantity, unit_price_cents)
VALUES
    ('64000000-0000-0000-0000-000000000001', '63000000-0000-0000-0000-000000000001', '62000000-0000-0000-0000-000000000001', 1, 12999),
    ('64000000-0000-0000-0000-000000000002', '63000000-0000-0000-0000-000000000001', '62000000-0000-0000-0000-000000000008', 1, 2999),
    ('64000000-0000-0000-0000-000000000003', '63000000-0000-0000-0000-000000000001', '62000000-0000-0000-0000-000000000002', 1, 6999),
    ('64000000-0000-0000-0000-000000000004', '63000000-0000-0000-0000-000000000002', '62000000-0000-0000-0000-000000000003', 1, 32999),
    ('64000000-0000-0000-0000-000000000005', '63000000-0000-0000-0000-000000000003', '62000000-0000-0000-0000-000000000006', 1, 24999),
    ('64000000-0000-0000-0000-000000000006', '63000000-0000-0000-0000-000000000003', '62000000-0000-0000-0000-000000000004', 1, 3999),
    ('64000000-0000-0000-0000-000000000007', '63000000-0000-0000-0000-000000000004', '62000000-0000-0000-0000-000000000007', 1, 8999),
    ('64000000-0000-0000-0000-000000000008', '63000000-0000-0000-0000-000000000004', '62000000-0000-0000-0000-000000000004', 1, 3999),
    ('64000000-0000-0000-0000-000000000009', '63000000-0000-0000-0000-000000000004', '62000000-0000-0000-0000-000000000008', 1, 2999),
    ('64000000-0000-0000-0000-000000000010', '63000000-0000-0000-0000-000000000005', '62000000-0000-0000-0000-000000000005', 1, 15999),
    ('64000000-0000-0000-0000-000000000011', '63000000-0000-0000-0000-000000000002', '62000000-0000-0000-0000-000000000010', 1, 4999),
    ('64000000-0000-0000-0000-000000000012', '63000000-0000-0000-0000-000000000003', '62000000-0000-0000-0000-000000000007', 1, 8999)
ON CONFLICT DO NOTHING;

INSERT INTO carts (id, user_id, items)
VALUES
    ('65000000-0000-0000-0000-000000000001', '51111111-1111-1111-1111-111111111111', '[{"product_id":"62000000-0000-0000-0000-000000000002","quantity":1},{"product_id":"62000000-0000-0000-0000-000000000008","quantity":2}]'::jsonb),
    ('65000000-0000-0000-0000-000000000002', '52222222-2222-2222-2222-222222222222', '[{"product_id":"62000000-0000-0000-0000-000000000006","quantity":1}]'::jsonb)
ON CONFLICT DO NOTHING;
`
}

// ClientCode returns a map of TypeScript files with typed helper functions for common ecommerce operations including product listing, cart management, and order creation.
func (ecommerceTemplate) ClientCode() map[string]string {
	return map[string]string{
		"src/lib/ecommerce.ts": `import { ayb } from "./ayb";

export interface Product {
  id: string;
  name: string;
  description: string;
  price_cents: number;
  currency: string;
  sku: string | null;
  stock_count: number;
  image_url: string | null;
  active: boolean;
  created_at: string;
}

export interface Customer {
  id: string;
  user_id: string;
  name: string;
  email: string;
  shipping_address: Record<string, unknown> | null;
  created_at: string;
}

export type OrderStatus =
  | "pending"
  | "paid"
  | "shipped"
  | "delivered"
  | "cancelled";

export interface Order {
  id: string;
  customer_id: string;
  status: OrderStatus;
  total_cents: number;
  created_at: string;
}

export interface OrderItem {
  id: string;
  order_id: string;
  product_id: string;
  quantity: number;
  unit_price_cents: number;
  created_at: string;
}

export interface CartItem {
  product_id: string;
  quantity: number;
}

export interface CreateOrderItemInput {
  product_id: string;
  quantity: number;
  unit_price_cents: number;
}

export function listProducts(filter?: string) {
  if (filter) {
    return ayb.records.list("products", { filter, sort: "name" });
  }
  return ayb.records.list("products", { sort: "name" });
}

export function getProduct(id: string) {
  return ayb.records.get("products", id);
}

export async function getCart() {
  const result = await ayb.records.list("carts", { limit: 1 });
  return result.items?.[0] ?? null;
}

export async function updateCart(items: CartItem[]) {
  const existing = await getCart();
  if (existing) {
    return ayb.records.update("carts", existing.id, {
      items,
      updated_at: new Date().toISOString(),
    });
  }

  const me = await ayb.auth.me();
  const userId = (me as { id?: string; user?: { id?: string } }).id
    ?? (me as { id?: string; user?: { id?: string } }).user?.id;
  if (!userId) {
    throw new Error("Cannot update cart without an authenticated user");
  }

  return ayb.records.create("carts", {
    user_id: userId,
    items,
  });
}

export async function createOrder(customerId: string, items: CreateOrderItemInput[]) {
  const totalCents = items.reduce(
    (sum, item) => sum + item.unit_price_cents * item.quantity,
    0
  );

  const order = await ayb.records.create("orders", {
    customer_id: customerId,
    status: "pending",
    total_cents: totalCents,
  });

  await Promise.all(
    items.map((item) =>
      ayb.records.create("order_items", {
        order_id: order.id,
        product_id: item.product_id,
        quantity: item.quantity,
        unit_price_cents: item.unit_price_cents,
      })
    )
  );

  return order;
}

export function listOrders(customerId?: string) {
  if (customerId) {
    return ayb.records.list("orders", {
      filter: "customer_id='" + customerId + "'",
      sort: "-created_at",
    });
  }
  return ayb.records.list("orders", { sort: "-created_at" });
}

export function getOrder(id: string) {
  return ayb.records.get("orders", id);
}
`,
	}
}

// Readme returns formatted markdown documentation for the ecommerce template, including schema overview, pricing conventions using integer cents, order status lifecycle, setup instructions, and SDK usage examples.
func (ecommerceTemplate) Readme() string {
	return `# Ecommerce Template

This scaffold provisions a production-style ecommerce schema and typed helper client code.

## Included schema

- ` + "`products`" + `: product catalog, inventory, active status, and SKU
- ` + "`customers`" + `: customer profile linked one-to-one with AYB auth users
- ` + "`orders`" + `: order header with lifecycle status and total in cents
- ` + "`order_items`" + `: per-product line items for each order
- ` + "`carts`" + `: per-user JSONB cart payload for pre-checkout state

## Pricing convention

All money values use integer cents (for example ` + "`12999`" + ` means $129.99 USD) to avoid floating-point rounding issues.

## Order status lifecycle

` + "`pending → paid → shipped → delivered`" + ` (or ` + "`cancelled`" + ` when an order does not complete).

## Apply schema and seed data

` + "```bash" + `
ayb sql < schema.sql && ayb sql < seed.sql
` + "```" + `

## SDK usage example

` + "```ts" + `
import { listProducts, updateCart, createOrder } from "./src/lib/ecommerce";

const { items: products } = await listProducts();
await updateCart([
  { product_id: products[0].id, quantity: 1 },
  { product_id: products[1].id, quantity: 2 },
]);

const order = await createOrder("<customer-id>", [
  { product_id: products[0].id, quantity: 1, unit_price_cents: products[0].price_cents },
  { product_id: products[1].id, quantity: 2, unit_price_cents: products[1].price_cents },
]);
console.log("created order", order.id);
` + "```" + `

## Quick start

1. Start AYB with ` + "`ayb start`" + `.
2. Apply schema and seed data.
3. Use ` + "`src/lib/ecommerce.ts`" + ` helpers to build catalog, cart, and order flows.
`
}
