import { describe, expect, it } from "vitest";

import {
  POSTGRES_PROVIDERS,
  getProviderById,
  detectProviderFromUrl,
  type ProviderInfo,
} from "./adapters";

describe("adapters", () => {
  describe("POSTGRES_PROVIDERS", () => {
    it("contains all expected providers", () => {
      const providerIds = POSTGRES_PROVIDERS.map((p) => p.id);
      expect(providerIds).toContain("postgres");
      expect(providerIds).toContain("supabase");
      expect(providerIds).toContain("neon");
      expect(providerIds).toContain("cockroachdb");
      expect(providerIds).toContain("yugabytedb");
      expect(providerIds).toContain("aurora");
      expect(providerIds).toContain("alloydb");
      expect(providerIds).toContain("timescale");
      expect(providerIds).toContain("redshift");
    });

    it("has exactly 9 providers", () => {
      expect(POSTGRES_PROVIDERS).toHaveLength(9);
    });

    it("all providers have adapterType postgres", () => {
      for (const provider of POSTGRES_PROVIDERS) {
        expect(provider.adapterType).toBe("postgres");
      }
    });

    it("all providers have required fields", () => {
      for (const provider of POSTGRES_PROVIDERS) {
        expect(provider.id).toBeTruthy();
        expect(provider.name).toBeTruthy();
        expect(typeof provider.defaultPort).toBe("number");
        expect(provider.defaultPort).toBeGreaterThan(0);
        expect(provider.defaultSSL).toBeTruthy();
      }
    });

    it("providers have correct default ports", () => {
      const portMap: Record<string, number> = {
        postgres: 5432,
        supabase: 6543,
        neon: 5432,
        cockroachdb: 26257,
        yugabytedb: 5433,
        aurora: 5432,
        alloydb: 5432,
        timescale: 5432,
        redshift: 5439,
      };

      for (const provider of POSTGRES_PROVIDERS) {
        expect(provider.defaultPort).toBe(portMap[provider.id]);
      }
    });

    it("cockroachdb uses verify-full SSL by default", () => {
      const cockroach = POSTGRES_PROVIDERS.find((p) => p.id === "cockroachdb");
      expect(cockroach?.defaultSSL).toBe("verify-full");
    });
  });

  describe("getProviderById", () => {
    it("returns provider for valid ID", () => {
      const provider = getProviderById("supabase");
      expect(provider).toBeDefined();
      expect(provider?.name).toBe("Supabase");
    });

    it("returns postgres provider for postgres ID", () => {
      const provider = getProviderById("postgres");
      expect(provider).toBeDefined();
      expect(provider?.name).toBe("PostgreSQL");
    });

    it("returns undefined for unknown ID", () => {
      const provider = getProviderById("unknown");
      expect(provider).toBeUndefined();
    });

    it("returns undefined for empty string", () => {
      const provider = getProviderById("");
      expect(provider).toBeUndefined();
    });
  });

  describe("detectProviderFromUrl", () => {
    it("detects Supabase from connection URL", () => {
      const url =
        "postgresql://postgres.abcdef:password@aws-0-us-west-1.pooler.supabase.com:6543/postgres";
      const provider = detectProviderFromUrl(url);
      expect(provider?.id).toBe("supabase");
    });

    it("detects Neon from connection URL", () => {
      const url =
        "postgresql://user:password@ep-cool-name.us-east-1.aws.neon.tech/database";
      const provider = detectProviderFromUrl(url);
      expect(provider?.id).toBe("neon");
    });

    it("detects CockroachDB from connection URL", () => {
      const url =
        "postgresql://user:password@cluster.us-west-2.cockroachlabs.cloud:26257/defaultdb";
      const provider = detectProviderFromUrl(url);
      expect(provider?.id).toBe("cockroachdb");
    });

    it("detects YugabyteDB from connection URL", () => {
      const url =
        "postgresql://user:password@us-east-1.xyz123.yugabyte.cloud:5433/yugabyte";
      const provider = detectProviderFromUrl(url);
      expect(provider?.id).toBe("yugabytedb");
    });

    it("detects Aurora from connection URL", () => {
      const url =
        "postgresql://user:password@myinstance.cluster-xyz123.us-east-1.rds.amazonaws.com:5432/mydb";
      const provider = detectProviderFromUrl(url);
      expect(provider?.id).toBe("aurora");
    });

    it("detects Redshift from connection URL", () => {
      const url =
        "postgresql://user:password@mycluster.xyz123.us-east-1.redshift.amazonaws.com:5439/dev";
      const provider = detectProviderFromUrl(url);
      expect(provider?.id).toBe("redshift");
    });

    it("detects TimescaleDB from timescaledb.io URL", () => {
      const url =
        "postgresql://user:password@service.region.timescaledb.io:5432/tsdb";
      const provider = detectProviderFromUrl(url);
      expect(provider?.id).toBe("timescale");
    });

    it("detects TimescaleDB from tsdb.cloud.timescale.com URL", () => {
      const url =
        "postgresql://user:password@tsdb.cloud.timescale.com:5432/tsdb";
      const provider = detectProviderFromUrl(url);
      expect(provider?.id).toBe("timescale");
    });

    it("returns undefined for generic localhost URL", () => {
      const url = "postgresql://user:password@localhost:5432/mydb";
      const provider = detectProviderFromUrl(url);
      expect(provider).toBeUndefined();
    });

    it("returns undefined for generic IP address URL", () => {
      const url = "postgresql://user:password@192.168.1.100:5432/mydb";
      const provider = detectProviderFromUrl(url);
      expect(provider).toBeUndefined();
    });

    it("returns undefined for empty string", () => {
      const provider = detectProviderFromUrl("");
      expect(provider).toBeUndefined();
    });

    it("is case-insensitive for hostname matching", () => {
      const url =
        "postgresql://user:password@AWS-0-US-WEST-1.POOLER.SUPABASE.COM:6543/postgres";
      const provider = detectProviderFromUrl(url);
      expect(provider?.id).toBe("supabase");
    });
  });

  describe("ProviderInfo interface", () => {
    it("allows optional fields to be undefined", () => {
      const minimalProvider: ProviderInfo = {
        id: "test",
        name: "Test Provider",
        icon: null,
        adapterType: "postgres",
        defaultPort: 5432,
        defaultSSL: "require",
      };
      expect(minimalProvider.urlPattern).toBeUndefined();
      expect(minimalProvider.helpUrl).toBeUndefined();
      expect(minimalProvider.connectionStringHelp).toBeUndefined();
    });
  });
});
