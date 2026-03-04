export interface User {
  id: string
  email: string
  display_name: string
  role: string
  created_at: string
  updated_at: string
}

export interface AppPassword {
  id: string
  user_id: string
  label: string
  last_used_at: string | null
  created_at: string
}

// ── Child record types (returned when relations are loaded) ─────────────────

export interface ContactEmail {
  id: string
  contact_id: string
  value: string
  type: string
  pref: number
  label: string
}

export interface ContactPhone {
  id: string
  contact_id: string
  value: string
  type: string
  pref: number
  label: string
}

export interface ContactAddress {
  id: string
  contact_id: string
  type: string
  pref: number
  label: string
  po_box: string
  extended: string
  street: string
  city: string
  region: string
  postal_code: string
  country: string
}

export interface ContactURL {
  id: string
  contact_id: string
  value: string
  type: string
  pref: number
}

export interface ContactIM {
  id: string
  contact_id: string
  value: string
  type: string
  pref: number
}

export interface ContactCategory {
  id: string
  contact_id: string
  value: string
}

export interface ContactDate {
  id: string
  contact_id: string
  kind: string
  value: string
  label: string
}

// ── Main Contact entity ─────────────────────────────────────────────────────

export interface Contact {
  id: string
  user_id: string
  address_book_id: string
  uid: string
  // Name
  first_name: string
  last_name: string
  middle_name: string
  name_prefix: string
  name_suffix: string
  nickname: string
  // Primary denormalised values (fast display)
  email: string
  phone: string
  // Organisation
  org: string
  department: string
  title: string
  role: string
  // Personal
  note: string
  bday: string
  anniversary: string
  gender: string
  tz: string
  geo: string
  photo_uri: string
  etag: string
  created_at: string
  updated_at: string
  // Relations — loaded on GetByID / List; absent on Search results
  emails?: ContactEmail[]
  phones?: ContactPhone[]
  addresses?: ContactAddress[]
  urls?: ContactURL[]
  ims?: ContactIM[]
  categories?: ContactCategory[]
  dates?: ContactDate[]
}

// ── Form data model (used by ContactForm.vue) ───────────────────────────────

export interface ContactFormField {
  value: string
  type: string
}

export interface ContactFormAddress {
  street: string
  city: string
  region: string
  postal_code: string
  country: string
  type: string
}

export interface ContactFormData {
  uid?: string
  first_name: string
  last_name: string
  middle_name: string
  name_prefix: string
  name_suffix: string
  nickname: string
  org: string
  department: string
  title: string
  role: string
  note: string
  bday: string
  anniversary: string
  gender: string
  tz: string
  emails: ContactFormField[]
  phones: ContactFormField[]
  urls: ContactFormField[]
  ims: ContactFormField[]
  addresses: ContactFormAddress[]
  categories: string[]
}

// ── API input types ─────────────────────────────────────────────────────────

export interface CreateContactInput {
  first_name: string
  last_name: string
  email: string
  phone: string
  org: string
  title: string
  note: string
  vcard_data?: string
}

export interface TokenPair {
  access_token: string
  refresh_token: string
}

export interface LoginInput {
  email: string
  password: string
}

export interface RegisterInput {
  email: string
  password: string
  display_name: string
}

export interface SyncRun {
  id: string
  user_id: string
  pipeline_id?: string
  provider_type: string
  status: 'running' | 'completed' | 'failed'
  created_count: number
  updated_count: number
  deleted_count: number
  error_count: number
  error_message?: string
  started_at: string
  finished_at?: string
}

export interface Pipeline {
  id: string
  user_id: string
  name: string
  enabled: boolean
  schedule: string
  steps: PipelineStep[]
  created_at: string
  updated_at: string
}

export interface PipelineStep {
  source_type: string
  dest_type: string
  conflict_mode: 'source_wins' | 'dest_wins' | 'skip' | 'auto' | 'manual'
  direction: 'pull' | 'push' | 'bidirectional'
  source_config?: string
  dest_config?: string
}

export interface PotentialDuplicate {
  id: string
  user_id: string
  contact_a_id: string
  contact_b_id: string
  score: number
  match_reasons: string // JSON-encoded string[]
  status: 'pending' | 'dismissed' | 'merged'
  created_at: string
  contact_a?: Contact
  contact_b?: Contact
}

export interface MergeInput {
  winner_id: string
  loser_id: string
  resolution: Record<string, string> // vCard field → "winner" | "loser"
}

export interface FieldDiff {
  field: string
  base: string
  local: string
  remote: string
}

export interface SyncConflict {
  id: string
  user_id: string
  provider_type: string
  remote_id: string
  local_contact_id: string
  base_vcard: string
  local_vcard: string
  remote_vcard: string
  field_diffs: string // JSON-encoded FieldDiff[]
  status: 'pending' | 'resolved' | 'dismissed'
  resolution: string
  resolved_vcard: string
  created_at: string
  resolved_at?: string
}

export interface CreatePipelineInput {
  name: string
  enabled: boolean
  schedule: string
  steps: PipelineStep[]
}

export interface BackupInfo {
  id: string
  filename: string
  size: number
  created_at: string
}

export interface BackupSettings {
  schedule: string
  retention: number
  enabled: boolean
  compress: boolean
}

export interface RestoreResult {
  imported: number
  skipped: number
  errors: number
}

export interface ImportResult {
  imported: number
  skipped: number
  errors: string[]
}

export interface SyncProvider {
  id: string
  provider_type: string
  name: string
  endpoint: string
  connected: boolean
  last_sync_at?: string
  last_error: string
  created_at: string
}

// Credential is a stored connection profile (same table as SyncProvider, cleaner name).
export interface Credential {
  id: string
  provider_type: string
  name: string
  endpoint: string
  username: string
  skip_tls_verify: boolean
  connected: boolean
  last_sync_at?: string
  last_error: string
  created_at: string
  updated_at: string
}

export interface PaginatedResponse<T> {
  data: T[]
  total: number
  page: number
  per_page: number
}

export interface ChangePasswordInput {
  old_password: string
  new_password: string
}

export interface UpdateProfileInput {
  display_name: string
  email: string
}

export interface UpdateRoleInput {
  role: string
}

export interface DedupSettings {
  schedule: string
  enabled: boolean
}
