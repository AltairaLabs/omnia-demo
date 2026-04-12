#!/usr/bin/env bash
# Seed the entire Acme Apparel demo store.
#
# Prerequisites:
#   shopify auth login
#   shopify store auth --store acme-apparel-omnia-demo.myshopify.com \
#     --scopes read_products,write_products,read_customers,write_customers,\
#     read_orders,write_orders,write_discounts,read_discounts,write_price_rules,read_price_rules
#
# Usage:
#   ./scripts/seed-data/create-all.sh
set -euo pipefail

STORE="acme-apparel-omnia-demo.myshopify.com"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

run_mutations() {
  local dir="$1" label="$2"
  echo "--- $label ---"
  local count=0 ok=0 fail=0
  for qf in "$dir"/*.graphql; do
    [[ -f "$qf" ]] || continue
    local name
    name=$(basename "$qf" .graphql)
    echo -n "  $name... "
    local output
    output=$(shopify store execute --store "$STORE" --query-file "$qf" --allow-mutations 2>&1) || true
    if echo "$output" | grep -q "Operation succeeded"; then
      echo "OK"
      ((ok++))
    else
      echo "FAIL"
      echo "$output" | grep -A2 '"message"' | head -5 | sed 's/^/    /'
      ((fail++))
    fi
    ((count++))
  done
  echo "  ($ok/$count succeeded, $fail failed)"
  echo ""
}

# Orders are special: draftOrderCreate returns a draft ID that must be
# completed with draftOrderComplete to become a real order.
run_draft_orders() {
  local dir="$1"
  echo "--- Orders (draft → complete) ---"
  local count=0 ok=0 fail=0
  for qf in "$dir"/*.graphql; do
    [[ -f "$qf" ]] || continue
    local name
    name=$(basename "$qf" .graphql)
    echo -n "  $name... "

    # Step 1: Create draft
    local draft_out
    draft_out=$(shopify store execute --store "$STORE" --json --query-file "$qf" --allow-mutations 2>/dev/null) || true
    local draft_id
    draft_id=$(echo "$draft_out" | python3 -c "
import sys, json
d = json.load(sys.stdin)
do = d.get('draftOrderCreate', {}).get('draftOrder')
print(do['id'] if do else '')
" 2>/dev/null) || true

    if [[ -z "$draft_id" ]]; then
      echo "FAIL (draft)"
      echo "$draft_out" | python3 -c "
import sys, json
d = json.load(sys.stdin)
for e in d.get('draftOrderCreate', {}).get('userErrors', []):
    print(f\"    {e.get('message','?')}\")
" 2>/dev/null || true
      ((fail++)); ((count++))
      continue
    fi

    # Step 2: Complete draft → real order
    local complete_out
    complete_out=$(shopify store execute --store "$STORE" --json \
      --query "mutation { draftOrderComplete(id: \"$draft_id\") { draftOrder { order { id name } } userErrors { field message } } }" \
      --allow-mutations 2>/dev/null) || true

    local order_name
    order_name=$(echo "$complete_out" | python3 -c "
import sys, json
d = json.load(sys.stdin)
o = d.get('draftOrderComplete', {}).get('draftOrder', {}).get('order')
print(o['name'] if o else '')
" 2>/dev/null) || true

    if [[ -n "$order_name" ]]; then
      echo "OK → $order_name"
      ((ok++))
    else
      echo "FAIL (complete)"
      echo "$complete_out" | python3 -c "
import sys, json
d = json.load(sys.stdin)
for e in d.get('draftOrderComplete', {}).get('userErrors', []):
    print(f\"    {e.get('message','?')}\")
" 2>/dev/null || true
      ((fail++))
    fi
    ((count++))
  done
  echo "  ($ok/$count succeeded, $fail failed)"
  echo ""
}

echo "=== Seeding Acme Apparel Demo Store ==="
echo "Store: $STORE"
echo ""

run_mutations "$SCRIPT_DIR/products" "Products"
run_mutations "$SCRIPT_DIR/customers" "Customers"
run_draft_orders "$SCRIPT_DIR/orders"

echo "=== Seed complete ==="
echo ""
echo "Verify:"
echo "  shopify store execute --store $STORE --query 'query { shop { name } products(first:25) { edges { node { title } } } customers(first:10) { edges { node { displayName email } } } orders(first:20) { edges { node { name displayFulfillmentStatus } } } }'"
