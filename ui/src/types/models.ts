/**
 * Core Data Models
 * These types match the database models from the Go backend
 */

export type ProjectStatus = 'active' | 'inactive' | 'deleted';
export type UserRole = 'admin' | 'data' | 'user';

export interface Project {
  id: string;
  name: string;
  parameters?: Record<string, unknown>;
  created_at: string;
  updated_at: string;
  status: ProjectStatus;
}

export interface User {
  project_id: string;
  user_id: string;
  role: UserRole;
  created_at: string;
  updated_at: string;
}

export interface CreateProjectRequest {
  name: string;
  parameters?: Record<string, unknown>;
}

export interface CreateProjectResponse {
  project: Project;
}

export interface AddUserRequest {
  user_id: string;
  role: UserRole;
}

export interface UpdateUserRequest {
  role: UserRole;
}

export interface RemoveUserRequest {
  user_id: string;
}
