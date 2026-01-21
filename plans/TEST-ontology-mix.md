# TEST: Ontology Extraction Mixed Scenarios

## Purpose

Test the ontology extraction pipeline against a variety of database relationship patterns to verify it correctly identifies entities, relationships, and semantic roles. This uses the `test_data` database in the Docker container.

## Database Connection

```bash
# Connect to test_data database in Docker
docker exec -it ekaya-engine-postgres-1 psql -U postgres -d test_data
```

## Test Scenarios

### Scenario 1: Classic Foreign Key Relationships

Standard FK constraints that should be automatically detected.

```sql
-- Parent entity
CREATE TABLE s1_customers (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    email VARCHAR(255) UNIQUE,
    created_at TIMESTAMP DEFAULT NOW()
);

-- Child with explicit FK
CREATE TABLE s1_orders (
    id SERIAL PRIMARY KEY,
    customer_id INTEGER NOT NULL REFERENCES s1_customers(id),
    order_date DATE NOT NULL,
    total_amount DECIMAL(10,2)
);

-- Grandchild with FK to orders
CREATE TABLE s1_order_items (
    id SERIAL PRIMARY KEY,
    order_id INTEGER NOT NULL REFERENCES s1_orders(id),
    product_name VARCHAR(100),
    quantity INTEGER,
    unit_price DECIMAL(10,2)
);
```

**Expected Ontology Results:**
- Entities: Customer, Order, OrderItem
- Relationships: Order → Customer (N:1), OrderItem → Order (N:1)
- Cardinality correctly inferred from FK constraints

### Scenario 2: UUID Primary Keys (No FK Constraints)

Common pattern in distributed systems - UUIDs without explicit FKs.

```sql
CREATE TABLE s2_users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username VARCHAR(50) NOT NULL,
    profile_data JSONB
);

CREATE TABLE s2_posts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    author_id UUID NOT NULL,  -- No FK constraint, but naming convention
    title VARCHAR(200),
    content TEXT,
    published_at TIMESTAMP
);

CREATE TABLE s2_comments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    post_id UUID NOT NULL,    -- Implicit relationship to posts
    commenter_id UUID,        -- Implicit relationship to users
    body TEXT,
    created_at TIMESTAMP DEFAULT NOW()
);
```

**Expected Ontology Results:**
- Entities: User, Post, Comment
- Relationships inferred from naming convention (*_id columns)
- Should detect: Post.author_id → User, Comment.post_id → Post, Comment.commenter_id → User

### Scenario 3: Composite Primary Keys

Tables using composite keys for many-to-many relationships.

```sql
CREATE TABLE s3_students (
    student_id INTEGER PRIMARY KEY,
    name VARCHAR(100),
    enrollment_year INTEGER
);

CREATE TABLE s3_courses (
    course_code VARCHAR(10) PRIMARY KEY,
    course_name VARCHAR(100),
    credits INTEGER
);

-- Junction table with composite PK
CREATE TABLE s3_enrollments (
    student_id INTEGER REFERENCES s3_students(student_id),
    course_code VARCHAR(10) REFERENCES s3_courses(course_code),
    enrolled_date DATE,
    grade CHAR(2),
    PRIMARY KEY (student_id, course_code)
);
```

**Expected Ontology Results:**
- Entities: Student, Course, Enrollment
- Relationships: Enrollment → Student (N:1), Enrollment → Course (N:1)
- Many-to-many relationship between Student and Course via Enrollment

### Scenario 4: Self-Referential Relationships

Tables that reference themselves (hierarchies, graphs).

```sql
CREATE TABLE s4_employees (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    manager_id INTEGER REFERENCES s4_employees(id),
    department VARCHAR(50),
    hire_date DATE
);

CREATE TABLE s4_categories (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100),
    parent_category_id INTEGER REFERENCES s4_categories(id),
    level INTEGER
);
```

**Expected Ontology Results:**
- Entity: Employee with self-referential relationship (manager)
- Entity: Category with self-referential relationship (parent)
- Role semantics: manager_id has role "manager", parent_category_id has role "parent"

### Scenario 5: Polymorphic Associations

Single column referencing multiple entity types (common in Rails/Django).

```sql
CREATE TABLE s5_users (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100)
);

CREATE TABLE s5_organizations (
    id SERIAL PRIMARY KEY,
    org_name VARCHAR(100)
);

-- Polymorphic: owner can be user or organization
CREATE TABLE s5_documents (
    id SERIAL PRIMARY KEY,
    title VARCHAR(200),
    owner_type VARCHAR(20) NOT NULL,  -- 'user' or 'organization'
    owner_id INTEGER NOT NULL,         -- References either table
    content TEXT
);

-- Activity log with polymorphic subject
CREATE TABLE s5_activities (
    id SERIAL PRIMARY KEY,
    subject_type VARCHAR(50),
    subject_id INTEGER,
    action VARCHAR(50),
    performed_at TIMESTAMP DEFAULT NOW()
);
```

**Expected Ontology Results:**
- Should detect polymorphic pattern from *_type/*_id pairs
- May not fully resolve without data inspection
- Document potential ambiguity in ontology questions

### Scenario 6: Soft Deletes and Audit Columns

Tables with common patterns for soft deletes and auditing.

```sql
CREATE TABLE s6_products (
    id SERIAL PRIMARY KEY,
    sku VARCHAR(50) UNIQUE,
    name VARCHAR(200),
    price DECIMAL(10,2),

    -- Soft delete
    is_deleted BOOLEAN DEFAULT FALSE,
    deleted_at TIMESTAMP,

    -- Audit columns
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    created_by INTEGER,
    updated_by INTEGER
);

CREATE TABLE s6_inventory (
    id SERIAL PRIMARY KEY,
    product_id INTEGER REFERENCES s6_products(id),
    warehouse_id INTEGER,
    quantity INTEGER,

    -- Same audit pattern
    is_deleted BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);
```

**Expected Ontology Results:**
- Should recognize soft delete pattern (is_deleted, deleted_at)
- Should recognize audit columns (created_at, updated_at, created_by, updated_by)
- created_by/updated_by may generate ontology questions about what entity they reference

### Scenario 7: Enum-Like Status Columns

Tables with status/type columns that have limited values.

```sql
CREATE TABLE s7_tickets (
    id SERIAL PRIMARY KEY,
    title VARCHAR(200),
    description TEXT,
    status VARCHAR(20) NOT NULL DEFAULT 'open',  -- open, in_progress, resolved, closed
    priority INTEGER DEFAULT 3,                   -- 1=critical, 2=high, 3=medium, 4=low
    ticket_type VARCHAR(30),                      -- bug, feature, question, task
    assignee_id INTEGER,
    created_at TIMESTAMP DEFAULT NOW()
);

-- Insert sample data to help enum detection
INSERT INTO s7_tickets (title, status, priority, ticket_type) VALUES
    ('Fix login bug', 'open', 1, 'bug'),
    ('Add dark mode', 'in_progress', 3, 'feature'),
    ('How to reset password?', 'resolved', 4, 'question'),
    ('Update docs', 'closed', 3, 'task');
```

**Expected Ontology Results:**
- Should detect status as enum with values: open, in_progress, resolved, closed
- Should detect priority as enum with values: 1, 2, 3, 4
- Should detect ticket_type as enum with values: bug, feature, question, task

### Scenario 8: Multiple Relationships to Same Entity

Different semantic roles pointing to the same entity.

```sql
CREATE TABLE s8_people (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100),
    email VARCHAR(255)
);

CREATE TABLE s8_contracts (
    id SERIAL PRIMARY KEY,
    contract_number VARCHAR(50),
    buyer_id INTEGER REFERENCES s8_people(id),
    seller_id INTEGER REFERENCES s8_people(id),
    witness_id INTEGER REFERENCES s8_people(id),
    amount DECIMAL(12,2),
    signed_date DATE
);

CREATE TABLE s8_messages (
    id SERIAL PRIMARY KEY,
    sender_id INTEGER REFERENCES s8_people(id),
    recipient_id INTEGER REFERENCES s8_people(id),
    subject VARCHAR(200),
    body TEXT,
    sent_at TIMESTAMP
);
```

**Expected Ontology Results:**
- Entity: Person (from s8_people)
- Entity: Contract with roles: buyer, seller, witness (all → Person)
- Entity: Message with roles: sender, recipient (both → Person)
- Should correctly identify different semantic roles

### Scenario 9: Natural Keys vs Surrogate Keys

Mix of ID styles in the same schema.

```sql
-- Natural key
CREATE TABLE s9_countries (
    country_code CHAR(2) PRIMARY KEY,  -- ISO 3166-1 alpha-2
    name VARCHAR(100) NOT NULL
);

-- Natural key
CREATE TABLE s9_currencies (
    currency_code CHAR(3) PRIMARY KEY,  -- ISO 4217
    name VARCHAR(100),
    symbol VARCHAR(5)
);

-- Surrogate key with natural key references
CREATE TABLE s9_exchange_rates (
    id SERIAL PRIMARY KEY,
    from_currency CHAR(3) REFERENCES s9_currencies(currency_code),
    to_currency CHAR(3) REFERENCES s9_currencies(currency_code),
    rate DECIMAL(15,6),
    effective_date DATE
);

-- Mix of both
CREATE TABLE s9_addresses (
    id SERIAL PRIMARY KEY,
    street VARCHAR(200),
    city VARCHAR(100),
    country_code CHAR(2) REFERENCES s9_countries(country_code),
    postal_code VARCHAR(20)
);
```

**Expected Ontology Results:**
- Should handle non-integer PKs correctly
- Should detect relationships via CHAR/VARCHAR FKs
- ExchangeRate has two relationships to Currency (from_currency, to_currency)

### Scenario 10: JSONB Columns with Embedded Relationships

Modern pattern with semi-structured data.

```sql
CREATE TABLE s10_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_type VARCHAR(50),
    payload JSONB NOT NULL,
    metadata JSONB,
    occurred_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE s10_user_preferences (
    user_id INTEGER PRIMARY KEY,
    preferences JSONB NOT NULL DEFAULT '{}',
    feature_flags JSONB DEFAULT '{}'
);
```

**Expected Ontology Results:**
- Should recognize JSONB columns
- May generate ontology questions about JSONB structure
- Should not try to infer relationships from JSONB content

### Scenario 11: Provenance Precedence Preservation

Test that ontology extraction respects the provenance hierarchy: **Manual > MCP > Inference**.
Higher-precedence items should never be deleted or overwritten by lower-precedence operations.

**Setup (via MCP tools before extraction):**

```bash
# 1. Create manual entity with custom description
update_entity(name='Customer', description='A paying customer who has completed at least one purchase', aliases=['client', 'buyer'])

# 2. Create manual relationship
update_relationship(from_entity='Order', to_entity='Customer', label='placed_by', description='The customer who placed this order')

# 3. Create glossary term
update_glossary_term(term='Active Customer', definition='Customer with purchase in last 90 days', sql='SELECT * FROM customers WHERE last_order_date > NOW() - INTERVAL 90 DAY')

# 4. Create project knowledge
update_project_knowledge(fact='Orders under $10 are considered micro-transactions', category='business_rule')

# 5. Create column metadata
update_column(table='s1_orders', column='total_amount', description='Order total in USD, excludes tax', enum_values=['micro:<10', 'small:10-100', 'medium:100-500', 'large:>500'])
```

**Test Execution:**

1. Run full ontology extraction via UI
2. Wait for all DAG steps to complete
3. Verify all manual items are preserved

**Verification Queries:**

```sql
-- Verify manual entity preserved with original description
SELECT name, description, created_by
FROM engine_ontology_entities
WHERE name = 'Customer' AND project_id = '<project-id>';
-- Expected: description = 'A paying customer...', created_by = 'mcp'

-- Verify manual aliases preserved
SELECT a.alias
FROM engine_ontology_entity_aliases a
JOIN engine_ontology_entities e ON a.entity_id = e.id
WHERE e.name = 'Customer';
-- Expected: 'client', 'buyer'

-- Verify manual relationship preserved
SELECT r.label, r.description, r.created_by
FROM engine_entity_relationships r
JOIN engine_ontology_entities se ON r.source_entity_id = se.id
JOIN engine_ontology_entities te ON r.target_entity_id = te.id
WHERE se.name = 'Order' AND te.name = 'Customer';
-- Expected: label = 'placed_by', description = 'The customer who placed...', created_by = 'mcp'

-- Verify glossary term preserved
SELECT term, definition, defining_sql
FROM engine_glossary_terms
WHERE term = 'Active Customer' AND project_id = '<project-id>';

-- Verify project knowledge preserved
SELECT fact, category
FROM engine_project_knowledge
WHERE project_id = '<project-id>' AND fact LIKE '%micro-transactions%';

-- Verify column metadata preserved
SELECT cm.description, cm.enum_values
FROM engine_column_metadata cm
JOIN engine_schema_columns c ON cm.column_id = c.id
JOIN engine_schema_tables t ON c.schema_table_id = t.id
WHERE t.table_name = 's1_orders' AND c.column_name = 'total_amount';
```

**Expected Ontology Results:**
- Manual/MCP entities NOT deleted during Entity Discovery
- Manual/MCP entity descriptions NOT overwritten by inference
- Manual/MCP aliases preserved alongside any new inferred aliases
- Manual/MCP relationships NOT deleted or modified
- Manual/MCP glossary terms preserved
- Manual/MCP project knowledge preserved
- Manual/MCP column metadata preserved
- New inference-created items coexist with manual items (no duplicates)

**Edge Cases to Test:**

1. **Same entity name**: If extraction discovers an entity with same name as manual entity, manual entity should be preserved (not duplicated or overwritten)
2. **Conflicting descriptions**: Manual description takes precedence over inferred description
3. **Re-extraction**: Running extraction multiple times should not accumulate duplicates or lose manual data

## Test Execution Steps

### Step 1: Create Test Tables

```bash
# Connect and create all test tables
docker exec -i ekaya-engine-postgres-1 psql -U postgres -d test_data << 'EOF'
-- Paste all CREATE TABLE statements from scenarios above
EOF
```

### Step 2: Add Test Data

```bash
# Insert sample data for enum detection and cardinality inference
docker exec -i ekaya-engine-postgres-1 psql -U postgres -d test_data << 'EOF'
-- Scenario 1: FK relationships
INSERT INTO s1_customers (name, email) VALUES
    ('Alice', 'alice@example.com'),
    ('Bob', 'bob@example.com');
INSERT INTO s1_orders (customer_id, order_date, total_amount) VALUES
    (1, '2024-01-15', 150.00),
    (1, '2024-02-20', 75.50),
    (2, '2024-01-10', 200.00);

-- Scenario 2: UUIDs
INSERT INTO s2_users (username) VALUES ('john_doe'), ('jane_smith');
-- Get UUIDs and use them for posts/comments

-- Scenario 7: Enums (already included above)

-- Add more test data as needed
EOF
```

### Step 3: Configure Datasource

1. In the UI, create a new datasource pointing to `test_data` database
2. Or use existing test datasource if available
3. Run schema sync to detect new tables

### Step 4: Run Ontology Extraction

1. Trigger ontology extraction via UI or API
2. Monitor DAG progress
3. Wait for completion

### Step 5: Verify Results

For each scenario, verify:

```sql
-- Check discovered entities
SELECT name, description, primary_table
FROM engine_ontology_entities
WHERE project_id = '<project-id>';

-- Check relationships
SELECT
    se.name as source_entity,
    te.name as target_entity,
    r.label,
    r.cardinality,
    r.description
FROM engine_entity_relationships r
JOIN engine_ontology_entities se ON r.source_entity_id = se.id
JOIN engine_ontology_entities te ON r.target_entity_id = te.id
WHERE r.project_id = '<project-id>';

-- Check entity occurrences (role detection)
SELECT
    e.name as entity,
    o.table_name,
    o.column_name,
    o.role
FROM engine_ontology_entity_occurrences o
JOIN engine_ontology_entities e ON o.entity_id = e.id
WHERE e.project_id = '<project-id>';

-- Check generated questions
SELECT category, question_text, context
FROM engine_ontology_questions
WHERE project_id = '<project-id>' AND status = 'pending';
```

## Expected Issues to Document

1. **UUID relationships without FKs** - May require PK-match detection
2. **Polymorphic associations** - May generate clarifying questions
3. **Self-referential** - Should detect but may need role clarification
4. **Multiple roles to same entity** - Critical test for role semantics
5. **JSONB columns** - Should be flagged but not deeply analyzed

## Cleanup

```sql
-- Drop all test tables
DROP TABLE IF EXISTS s1_order_items, s1_orders, s1_customers CASCADE;
DROP TABLE IF EXISTS s2_comments, s2_posts, s2_users CASCADE;
DROP TABLE IF EXISTS s3_enrollments, s3_courses, s3_students CASCADE;
DROP TABLE IF EXISTS s4_employees, s4_categories CASCADE;
DROP TABLE IF EXISTS s5_activities, s5_documents, s5_organizations, s5_users CASCADE;
DROP TABLE IF EXISTS s6_inventory, s6_products CASCADE;
DROP TABLE IF EXISTS s7_tickets CASCADE;
DROP TABLE IF EXISTS s8_messages, s8_contracts, s8_people CASCADE;
DROP TABLE IF EXISTS s9_addresses, s9_exchange_rates, s9_currencies, s9_countries CASCADE;
DROP TABLE IF EXISTS s10_user_preferences, s10_events CASCADE;
```

## Success Criteria

### Schema Detection
- [ ] All FK-based relationships detected with correct cardinality
- [ ] UUID relationships inferred from naming conventions
- [ ] Composite PKs handled correctly
- [ ] Self-referential relationships detected
- [ ] Multiple roles to same entity distinguished (buyer vs seller vs witness)
- [ ] Enum values detected for status/type columns
- [ ] Audit columns (created_at, etc.) recognized
- [ ] Appropriate ontology questions generated for ambiguous cases
- [ ] No false positive relationships
- [ ] Entity names are semantically meaningful (not just table names)

### Provenance Precedence (Manual > MCP > Inference)
- [ ] Manual/MCP entities preserved during extraction (not deleted)
- [ ] Manual/MCP entity descriptions not overwritten by inference
- [ ] Manual/MCP aliases preserved
- [ ] Manual/MCP relationships preserved (not deleted or modified)
- [ ] Manual/MCP glossary terms preserved
- [ ] Manual/MCP project knowledge preserved
- [ ] Manual/MCP column metadata preserved
- [ ] No duplicate entities created (same name, different provenance)
- [ ] Re-extraction is idempotent for manual items
