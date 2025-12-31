/**
 * Entity Types
 * Types for ontology entity management
 */

/**
 * EntityOccurrence represents a column occurrence of an entity.
 */
export interface EntityOccurrence {
  id: string;
  schema_name: string;
  table_name: string;
  column_name: string;
  role: string | null;
  confidence: number;
}

/**
 * EntityAlias represents an alias for an entity.
 */
export interface EntityAlias {
  id: string;
  alias: string;
  source: string | null;
}

/**
 * EntityDetail represents an entity with full details.
 */
export interface EntityDetail {
  id: string;
  name: string;
  description: string;
  primary_schema: string;
  primary_table: string;
  primary_column: string;
  occurrences: EntityOccurrence[];
  aliases: EntityAlias[];
  occurrence_count: number;
  is_deleted: boolean;
  deletion_reason?: string | null;
}

/**
 * EntitiesListResponse for GET /entities endpoint.
 */
export interface EntitiesListResponse {
  entities: EntityDetail[];
  total: number;
}
