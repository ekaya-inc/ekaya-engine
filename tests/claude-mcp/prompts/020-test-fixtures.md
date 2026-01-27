# Test: Create Test Fixtures

Create test tables and data that subsequent tests depend on.

## Tools Under Test

- `execute`

## Purpose

This test creates the `mcp_test_*` tables and populates them with known data
so that read operation tests (100-series) have predictable data to work with.

## Test Cases

### 1. Create Test Table
Call `execute` to create the main test table:

```sql
CREATE TABLE IF NOT EXISTS mcp_test_users (
  id SERIAL PRIMARY KEY,
  name VARCHAR(100) NOT NULL,
  email VARCHAR(255) UNIQUE,
  status VARCHAR(20) DEFAULT 'active',
  score INTEGER,
  created_at TIMESTAMP DEFAULT NOW()
)
```

Verify:
- Table is created
- Appears in `get_schema`

### 2. Create Related Test Table
Call `execute` to create a related table for relationship testing:

```sql
CREATE TABLE IF NOT EXISTS mcp_test_orders (
  id SERIAL PRIMARY KEY,
  user_id INTEGER REFERENCES mcp_test_users(id),
  amount DECIMAL(10,2),
  status VARCHAR(20) DEFAULT 'pending',
  created_at TIMESTAMP DEFAULT NOW()
)
```

Verify:
- Table is created
- FK constraint exists

### 3. Insert Test Data - Users
Call `execute` to insert known test data:

```sql
INSERT INTO mcp_test_users (name, email, status, score) VALUES
  ('Alice Test', 'alice@mcp-test.example', 'active', 100),
  ('Bob Test', 'bob@mcp-test.example', 'active', 85),
  ('Carol Test', 'carol@mcp-test.example', 'inactive', 90),
  ('Dave Test', NULL, 'active', NULL)
```

Verify:
- 4 rows inserted
- Data queryable via `query` tool

### 4. Insert Test Data - Orders
Call `execute` to insert order data:

```sql
INSERT INTO mcp_test_orders (user_id, amount, status) VALUES
  (1, 99.99, 'completed'),
  (1, 149.50, 'pending'),
  (2, 25.00, 'completed'),
  (3, 200.00, 'cancelled')
```

Verify:
- 4 rows inserted
- FK references valid

### 5. Verify Test Data
Call `query` to verify data is accessible:

```sql
SELECT COUNT(*) as user_count FROM mcp_test_users
```

```sql
SELECT COUNT(*) as order_count FROM mcp_test_orders
```

## Report Format

```
=== 020-test-fixtures: Create Test Fixtures ===

Test 1: Create Test Table (mcp_test_users)
  Created: [yes/no]
  In schema: [yes/no]
  RESULT: [PASS/FAIL]

Test 2: Create Related Test Table (mcp_test_orders)
  Created: [yes/no]
  FK constraint: [yes/no]
  RESULT: [PASS/FAIL]

Test 3: Insert Test Data - Users
  Rows inserted: [count]
  RESULT: [PASS/FAIL]

Test 4: Insert Test Data - Orders
  Rows inserted: [count]
  RESULT: [PASS/FAIL]

Test 5: Verify Test Data
  User count: [count]
  Order count: [count]
  RESULT: [PASS/FAIL]

OVERALL: [PASS/FAIL]
```

## Cleanup

Tables created here will be dropped in `900-cleanup.md`.
