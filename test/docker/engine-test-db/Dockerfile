FROM postgres:18-alpine

# Set environment variables for database initialization
ENV POSTGRES_DB=test_data
ENV POSTGRES_USER=ekaya
ENV POSTGRES_PASSWORD=test_password

# Copy the test database dump
COPY test_data.dump /docker-entrypoint-initdb.d/01-test_data.dump

# Create initialization script that will restore the dump and create additional databases
RUN echo '#!/bin/sh' > /docker-entrypoint-initdb.d/00-restore.sh && \
    echo 'set -e' >> /docker-entrypoint-initdb.d/00-restore.sh && \
    echo 'echo "Restoring test database..."' >> /docker-entrypoint-initdb.d/00-restore.sh && \
    echo 'pg_restore -U "$POSTGRES_USER" -d "$POSTGRES_DB" --no-owner --no-acl /docker-entrypoint-initdb.d/01-test_data.dump 2>&1 | grep -v "role.*does not exist" || true' >> /docker-entrypoint-initdb.d/00-restore.sh && \
    echo 'echo "Test database restored successfully"' >> /docker-entrypoint-initdb.d/00-restore.sh && \
    echo 'echo "Creating empty_db for empty database tests..."' >> /docker-entrypoint-initdb.d/00-restore.sh && \
    echo 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "CREATE DATABASE empty_db;"' >> /docker-entrypoint-initdb.d/00-restore.sh && \
    echo 'echo "empty_db created successfully"' >> /docker-entrypoint-initdb.d/00-restore.sh && \
    echo 'echo "Creating ekaya_engine_test for engine integration tests..."' >> /docker-entrypoint-initdb.d/00-restore.sh && \
    echo 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "CREATE DATABASE ekaya_engine_test;"' >> /docker-entrypoint-initdb.d/00-restore.sh && \
    echo 'echo "ekaya_engine_test created successfully"' >> /docker-entrypoint-initdb.d/00-restore.sh && \
    echo 'echo "EKAYA_TEST_DB_READY"' >> /docker-entrypoint-initdb.d/00-restore.sh && \
    chmod +x /docker-entrypoint-initdb.d/00-restore.sh

# Expose PostgreSQL port
EXPOSE 5432

# Health check
HEALTHCHECK --interval=5s --timeout=5s --retries=5 \
    CMD pg_isready -U ekaya -d test_data || exit 1
