package templates

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestEcommerceSchemaContainsAllTables(t *testing.T) {
	t.Parallel()
	dt := mustEcommerceTemplate(t)

	schema := dt.Schema()
	testutil.Contains(t, schema, "CREATE TABLE IF NOT EXISTS products")
	testutil.Contains(t, schema, "CREATE TABLE IF NOT EXISTS customers")
	testutil.Contains(t, schema, "CREATE TABLE IF NOT EXISTS orders")
	testutil.Contains(t, schema, "CREATE TABLE IF NOT EXISTS order_items")
	testutil.Contains(t, schema, "CREATE TABLE IF NOT EXISTS carts")
}

func TestEcommerceSchemaContainsRLS(t *testing.T) {
	t.Parallel()
	dt := mustEcommerceTemplate(t)

	schema := dt.Schema()
	testutil.Contains(t, schema, "products_select")
	testutil.Contains(t, schema, "products_insert")
	testutil.Contains(t, schema, "customers_select")
	testutil.Contains(t, schema, "orders_select")
	testutil.Contains(t, schema, "order_items_select")
	testutil.Contains(t, schema, "carts_select")
}

func TestEcommerceSchemaUsesUUIDSessionCast(t *testing.T) {
	t.Parallel()
	dt := mustEcommerceTemplate(t)

	schema := dt.Schema()
	testutil.Contains(t, schema, "current_setting('ayb.user_id', true)::uuid")
	testutil.False(t, strings.Contains(schema, "::text"))
}

func TestEcommerceSeedDataContainsExpectedInserts(t *testing.T) {
	t.Parallel()
	dt := mustEcommerceTemplate(t)

	seed := dt.SeedData()
	testutil.Contains(t, seed, "INSERT INTO products")
	testutil.Contains(t, seed, "INSERT INTO customers")
	testutil.Contains(t, seed, "INSERT INTO orders")
	testutil.Contains(t, seed, "INSERT INTO order_items")
	testutil.Contains(t, seed, "INSERT INTO carts")
}

func TestEcommerceClientCodeContainsHelpers(t *testing.T) {
	t.Parallel()
	dt := mustEcommerceTemplate(t)

	files := dt.ClientCode()
	code, ok := files["src/lib/ecommerce.ts"]
	testutil.True(t, ok)
	testutil.Contains(t, code, "listProducts")
	testutil.Contains(t, code, "createOrder")
	testutil.Contains(t, code, "getCart")
}

func TestEcommerceSeedOrderTotalsMatchOrderItems(t *testing.T) {
	t.Parallel()
	dt := mustEcommerceTemplate(t)

	seed := dt.SeedData()
	ordersBlock := extractInsertBlock(t, seed, "orders")
	orderItemsBlock := extractInsertBlock(t, seed, "order_items")

	orderTotals := make(map[string]int)
	orderRe := regexp.MustCompile(`\('([0-9a-f-]+)',\s*'([0-9a-f-]+)',\s*'([a-z]+)',\s*([0-9]+)\)`)
	for _, m := range orderRe.FindAllStringSubmatch(ordersBlock, -1) {
		total, err := strconv.Atoi(m[4])
		testutil.NoError(t, err)
		orderTotals[m[1]] = total
	}
	testutil.True(t, len(orderTotals) > 0)

	itemSums := make(map[string]int)
	itemRe := regexp.MustCompile(`\('([0-9a-f-]+)',\s*'([0-9a-f-]+)',\s*'([0-9a-f-]+)',\s*([0-9]+),\s*([0-9]+)\)`)
	for _, m := range itemRe.FindAllStringSubmatch(orderItemsBlock, -1) {
		qty, err := strconv.Atoi(m[4])
		testutil.NoError(t, err)
		unitPrice, err := strconv.Atoi(m[5])
		testutil.NoError(t, err)
		itemSums[m[2]] += qty * unitPrice
	}

	testutil.Equal(t, len(orderTotals), len(itemSums))
	for orderID, expectedTotal := range orderTotals {
		testutil.Equal(t, expectedTotal, itemSums[orderID])
	}
}

func extractInsertBlock(t *testing.T, seed, table string) string {
	t.Helper()
	start := strings.Index(seed, "INSERT INTO "+table)
	if start == -1 {
		t.Fatalf("missing INSERT INTO block for table %q", table)
	}
	rest := seed[start:]
	end := strings.Index(rest, "ON CONFLICT DO NOTHING;")
	if end == -1 {
		t.Fatalf("missing ON CONFLICT DO NOTHING terminator for table %q", table)
	}
	return rest[:end]
}

func mustEcommerceTemplate(t *testing.T) DomainTemplate {
	t.Helper()
	return mustTemplate(t, "ecommerce")
}
