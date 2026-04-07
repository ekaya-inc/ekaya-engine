export interface SetupStatus {
  steps: Record<string, boolean>;
  incomplete_count: number;
  next_step?: string;
}
