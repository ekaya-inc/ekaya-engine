#!/bin/bash
set -e

PGDATA=/var/lib/postgresql/data

echo "=== Ekaya Engine Quickstart ==="

# Initialize Postgres if data directory is empty (first run)
if [ -z "$(ls -A $PGDATA 2>/dev/null)" ]; then
    echo "First run detected. Initializing PostgreSQL..."

    # Ensure postgres owns the data directory
    chown -R postgres:postgres $PGDATA

    # Initialize the database cluster
    su postgres -c "/usr/lib/postgresql/17/bin/initdb -D $PGDATA"

    # Configure pg_hba.conf for local trust auth
    cat > $PGDATA/pg_hba.conf << 'EOF'
local   all             all                                     trust
host    all             all             127.0.0.1/32            trust
host    all             all             ::1/128                 trust
EOF

    # Configure postgresql.conf
    cat >> $PGDATA/postgresql.conf << 'EOF'
listen_addresses = 'localhost'
port = 5432
EOF

    # Start Postgres temporarily to create user and database
    echo "Starting PostgreSQL to create user and database..."
    su postgres -c "/usr/lib/postgresql/17/bin/pg_ctl -D $PGDATA -l /tmp/pg_init.log start"

    # Wait for Postgres to be ready (connect as postgres user to postgres database)
    until su postgres -c "PGUSER=postgres PGDATABASE=postgres pg_isready -q"; do
        echo "Waiting for PostgreSQL to start..."
        sleep 1
    done

    # Create ekaya user and database (override PG* vars to connect as postgres superuser to postgres db)
    echo "Creating ekaya user and ekaya_engine database..."
    su postgres -c "PGUSER=postgres PGDATABASE=postgres psql -c \"CREATE USER ekaya WITH PASSWORD 'quickstart';\""
    su postgres -c "PGUSER=postgres PGDATABASE=postgres psql -c \"CREATE DATABASE ekaya_engine OWNER ekaya;\""
    su postgres -c "PGUSER=postgres PGDATABASE=postgres psql -c \"GRANT ALL PRIVILEGES ON DATABASE ekaya_engine TO ekaya;\""

    # Stop Postgres (we'll start it properly below)
    echo "Database initialization complete. Restarting PostgreSQL..."
    su postgres -c "/usr/lib/postgresql/17/bin/pg_ctl -D $PGDATA stop"

    echo "=== First-run initialization complete ==="
fi

# Ensure postgres owns the data directory (for volume mounts)
chown -R postgres:postgres $PGDATA

# Start PostgreSQL
echo "Starting PostgreSQL..."
su postgres -c "/usr/lib/postgresql/17/bin/postgres -D $PGDATA" &
PG_PID=$!

# Start Redis
echo "Starting Redis..."
redis-server --bind 127.0.0.1 --port 6379 --daemonize no &
REDIS_PID=$!

# Wait for PostgreSQL to be ready
echo "Waiting for PostgreSQL to be ready..."
until su postgres -c "PGUSER=postgres PGDATABASE=postgres pg_isready -q"; do
    sleep 1
done
echo "PostgreSQL is ready."

# Wait for Redis to be ready
echo "Waiting for Redis to be ready..."
until redis-cli -h 127.0.0.1 -p 6379 ping > /dev/null 2>&1; do
    sleep 1
done
echo "Redis is ready."

# Trap signals to gracefully shutdown
cleanup() {
    echo "Shutting down..."
    kill $REDIS_PID 2>/dev/null || true
    su postgres -c "/usr/lib/postgresql/17/bin/pg_ctl -D $PGDATA stop -m fast" 2>/dev/null || true
    exit 0
}
trap cleanup SIGTERM SIGINT

echo "=== Starting Ekaya Engine ==="
echo "Open http://localhost:3443 in your browser"
echo ""

# Run ekaya-engine in foreground
# Using exec would replace the shell, but we need the trap handler
# So we run it and wait
/usr/local/bin/ekaya-engine &
ENGINE_PID=$!

# Wait for any process to exit
wait -n $PG_PID $REDIS_PID $ENGINE_PID

# If we get here, something exited
echo "A process exited unexpectedly. Shutting down..."
cleanup
