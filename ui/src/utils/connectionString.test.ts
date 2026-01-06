import { describe, expect, it } from 'vitest';

import { parsePostgresUrl } from './connectionString';

describe('connectionString', () => {
  describe('parsePostgresUrl', () => {
    it('parses a standard postgresql:// URL', () => {
      const result = parsePostgresUrl(
        'postgresql://user:password@localhost:5432/mydb'
      );

      expect(result).toEqual({
        host: 'localhost',
        port: 5432,
        user: 'user',
        password: 'password',
        database: 'mydb',
        sslMode: 'require',
        provider: undefined,
      });
    });

    it('parses a postgres:// URL (alternative scheme)', () => {
      const result = parsePostgresUrl(
        'postgres://user:password@localhost:5432/mydb'
      );

      expect(result).toEqual({
        host: 'localhost',
        port: 5432,
        user: 'user',
        password: 'password',
        database: 'mydb',
        sslMode: 'require',
        provider: undefined,
      });
    });

    it('extracts sslmode from query parameters', () => {
      const result = parsePostgresUrl(
        'postgresql://user:password@localhost:5432/mydb?sslmode=verify-full'
      );

      expect(result?.sslMode).toBe('verify-full');
    });

    it('defaults sslMode to require when not specified', () => {
      const result = parsePostgresUrl(
        'postgresql://user:password@localhost:5432/mydb'
      );

      expect(result?.sslMode).toBe('require');
    });

    it('handles URL without port (uses default 5432)', () => {
      const result = parsePostgresUrl(
        'postgresql://user:password@localhost/mydb'
      );

      expect(result?.port).toBe(5432);
    });

    it('handles URL without database', () => {
      const result = parsePostgresUrl(
        'postgresql://user:password@localhost:5432'
      );

      expect(result?.database).toBe('');
    });

    it('handles URL without user/password', () => {
      const result = parsePostgresUrl('postgresql://localhost:5432/mydb');

      expect(result?.user).toBe('');
      expect(result?.password).toBe('');
    });

    it('handles URL with user but no password', () => {
      const result = parsePostgresUrl('postgresql://user@localhost:5432/mydb');

      expect(result?.user).toBe('user');
      expect(result?.password).toBe('');
    });

    it('decodes URL-encoded characters in user/password', () => {
      const result = parsePostgresUrl(
        'postgresql://user%40domain:pass%23word@localhost:5432/mydb'
      );

      expect(result?.user).toBe('user@domain');
      expect(result?.password).toBe('pass#word');
    });

    it('returns null for invalid URLs', () => {
      expect(parsePostgresUrl('not-a-url')).toBeNull();
      expect(parsePostgresUrl('http://localhost:5432/mydb')).toBeNull();
      expect(parsePostgresUrl('mysql://localhost:3306/mydb')).toBeNull();
      expect(parsePostgresUrl('')).toBeNull();
    });

    // Provider auto-detection tests
    describe('provider auto-detection', () => {
      it('detects Supabase from URL and uses default port 6543', () => {
        const result = parsePostgresUrl(
          'postgresql://postgres.abcdefgh:password@aws-0-us-west-1.pooler.supabase.com/postgres'
        );

        expect(result?.provider).toBe('supabase');
        expect(result?.port).toBe(6543); // Supabase pooler default
        expect(result?.host).toBe('aws-0-us-west-1.pooler.supabase.com');
      });

      it('uses explicit port when provided for Supabase', () => {
        const result = parsePostgresUrl(
          'postgresql://postgres.abcdefgh:password@aws-0-us-west-1.pooler.supabase.com:5432/postgres'
        );

        expect(result?.provider).toBe('supabase');
        expect(result?.port).toBe(5432); // Explicit port overrides default
      });

      it('detects Neon from URL', () => {
        const result = parsePostgresUrl(
          'postgresql://user:password@ep-cool-darkness-123456.us-east-1.aws.neon.tech/neondb?sslmode=require'
        );

        expect(result?.provider).toBe('neon');
        expect(result?.sslMode).toBe('require');
      });

      it('detects CockroachDB from URL and uses default port 26257', () => {
        const result = parsePostgresUrl(
          'postgresql://user:password@cluster-name.gcp-us-central1.cockroachlabs.cloud/defaultdb'
        );

        expect(result?.provider).toBe('cockroachdb');
        expect(result?.port).toBe(26257);
      });

      it('detects YugabyteDB from URL', () => {
        const result = parsePostgresUrl(
          'postgresql://admin:password@us-west1.abc123.yugabyte.cloud/yugabyte'
        );

        expect(result?.provider).toBe('yugabytedb');
        expect(result?.port).toBe(5433); // YugabyteDB default
      });

      it('detects Aurora from RDS URL', () => {
        const result = parsePostgresUrl(
          'postgresql://admin:password@mydb.cluster-xyz.us-east-1.rds.amazonaws.com/postgres'
        );

        expect(result?.provider).toBe('aurora');
      });

      it('detects TimescaleDB from URL', () => {
        const result = parsePostgresUrl(
          'postgresql://tsdbadmin:password@abc123.tsdb.cloud.timescale.com/tsdb'
        );

        expect(result?.provider).toBe('timescale');
      });

      it('detects Redshift from URL', () => {
        const result = parsePostgresUrl(
          'postgresql://admin:password@my-cluster.abc123.us-east-1.redshift.amazonaws.com:5439/dev'
        );

        expect(result?.provider).toBe('redshift');
        expect(result?.port).toBe(5439);
      });

      it('returns undefined provider for unrecognized hosts', () => {
        const result = parsePostgresUrl(
          'postgresql://user:password@my-custom-server.example.com:5432/mydb'
        );

        expect(result?.provider).toBeUndefined();
        expect(result?.port).toBe(5432);
      });
    });

    // Edge cases
    describe('edge cases', () => {
      it('handles complex Supabase username with dots', () => {
        const result = parsePostgresUrl(
          'postgresql://postgres.abcdefghijklmnop:mypassword@aws-0-us-west-1.pooler.supabase.com:6543/postgres'
        );

        expect(result?.user).toBe('postgres.abcdefghijklmnop');
        expect(result?.password).toBe('mypassword');
      });

      it('handles multiple query parameters', () => {
        const result = parsePostgresUrl(
          'postgresql://user:password@localhost:5432/mydb?sslmode=verify-full&connect_timeout=10'
        );

        expect(result?.sslMode).toBe('verify-full');
      });

      it('handles empty password (colon present)', () => {
        const result = parsePostgresUrl(
          'postgresql://user:@localhost:5432/mydb'
        );

        expect(result?.user).toBe('user');
        expect(result?.password).toBe('');
      });
    });
  });
});
