// Shared JSON shapes returned by the seven Olympus services, as seen over the
// console gateway at /api/<service>/*.

// ---- Common -----------------------------------------------------------------

export interface ApiProject {
  id: string
  account_id: string
  name: string
  description: string
  created_at: string
  status: string
  // per-service extra counters:
  parameter_count?: number
  instance_count?: number
  cluster_count?: number
  queue_count?: number
  topic_count?: number
}

// ---- Paramdora --------------------------------------------------------------

export const PARAM_TYPES = ['string', 'string_list', 'secure_string'] as const
export type ParamType = (typeof PARAM_TYPES)[number]

export interface Parameter {
  id: string
  project_id: string
  name: string
  value: string
  data_type: string
  description: string
  is_encrypted: boolean
  tier: string
  version: number
  key_id: string
  tags: Record<string, string>
  created_at: string
  updated_at: string
  last_modified_by: string
  status: string
}

export interface ParameterPut {
  value: string
  type?: string
  description?: string
  tier?: string
  tags?: Record<string, string>
}

export interface HistoryResponse {
  versions: Parameter[]
}

// ---- Hephaestus -------------------------------------------------------------

export interface InstanceType {
  name: string
  vcpus: number
  memory_gb: number
  storage_gb: number
  price_per_hour_cents: number
  status: string
}

export interface Instance {
  id: string
  project_id: string
  name: string
  instance_type: string
  image_id: string
  state: string
  private_ip?: string
  public_ip?: string
  key_pair_name?: string
  provider_ref?: string
  metadata?: Record<string, string>
  launched_by: string
  launched_at?: string
  terminated_at?: string
  created_at: string
  updated_at: string
}

export interface KeyPair {
  id: string
  project_id: string
  name: string
  fingerprint: string
  public_key: string
  created_at: string
}

export interface KeyPairCreated extends KeyPair {
  private_key: string
}

export interface SecurityRule {
  port: number
  cidr: string
}

export interface SecurityGroup {
  id: string
  project_id: string
  name: string
  description: string
  rules: SecurityRule[]
  created_at: string
}

export interface Volume {
  id: string
  project_id: string
  name: string
  instance_id?: string
  size_gb: number
  volume_type: string
  state: string
  created_at: string
  updated_at: string
}

export interface Snapshot {
  id: string
  project_id: string
  name: string
  volume_id?: string
  size_gb: number
  state: string
  provider_ref?: string
  created_at: string
  updated_at: string
}

// ---- Orpheus ----------------------------------------------------------------

export interface KubernetesVersion {
  version: string
  channel: string
  status: string
}

export interface NodeSize {
  name: string
  vcpus: number
  memory_gb: number
  price_per_hour_cents: number
  status: string
}

export interface Cluster {
  id: string
  project_id: string
  name: string
  kubernetes_version: string
  region: string
  state: string
  endpoint?: string
  kubeconfig?: string
  provider_ref?: string
  created_at: string
  updated_at: string
  status: string
}

export interface NodeGroup {
  id: string
  cluster_id: string
  name: string
  node_size: string
  min_size: number
  desired_size: number
  max_size: number
  state: string
  provider_ref?: string
  created_at: string
  updated_at: string
  status: string
}

// ---- Clio -------------------------------------------------------------------

export interface DatabaseEngine {
  engine: string
  version: string
  status: string
}

export interface InstanceSize {
  name: string
  vcpus: number
  memory_gb: number
  storage_gb: number
  price_per_hour_cents: number
  status: string
}

export interface DBInstance {
  id: string
  project_id: string
  name: string
  engine: string
  engine_version: string
  size: string
  allocated_storage_gb: number
  state: string
  endpoint?: string
  master_username?: string
  master_password?: string
  provider_ref?: string
  created_at: string
  updated_at: string
  status: string
}

export interface DBSnapshot {
  id: string
  project_id: string
  instance_id: string
  instance?: string
  name: string
  size_gb: number
  state: string
  provider_ref?: string
  created_at: string
  updated_at: string
  status: string
}

// ---- Mneme ------------------------------------------------------------------

export interface CacheEngine {
  engine: string
  version: string
  status: string
}

export interface NodeType {
  name: string
  vcpus: number
  memory_gb: number
  price_per_hour_cents: number
  status: string
}

export interface CacheCluster {
  id: string
  project_id: string
  name: string
  engine: string
  engine_version: string
  node_type: string
  num_nodes: number
  state: string
  endpoint?: string
  provider_ref?: string
  created_at: string
  updated_at: string
  status: string
}

export interface CacheSnapshot {
  id: string
  project_id: string
  cluster_id: string
  name: string
  size_mb: number
  state: string
  provider_ref?: string
  created_at: string
  updated_at: string
  status: string
}

// ---- Iris -------------------------------------------------------------------

export interface Queue {
  id: string
  project_id: string
  name: string
  visibility_timeout_sec: number
  message_retention_sec: number
  state: string
  created_at: string
  updated_at: string
  status: string
}

export interface QueueMessage {
  id: string
  queue_id: string
  body: string
  attributes?: Record<string, string>
  state: string
  visible_at: string
  created_at: string
}

export interface Topic {
  id: string
  project_id: string
  name: string
  state: string
  created_at: string
  updated_at: string
  status: string
}

export interface Subscriber {
  id: string
  topic_id: string
  kind: string
  queue_id?: string
  queue_name?: string
  webhook_url?: string
  status: string
  created_at: string
  updated_at: string
}

export interface PublishResult {
  topic: string
  queue_copies: number
  webhook_deliveries: number
}

// ---- Themis (IAM) -----------------------------------------------------------

export interface ThemisUser {
  id: string
  project_id: string
  name: string
  description: string
  path: string
  tags: Record<string, string>
  created_at: string
  updated_at: string
  status: string
}

export interface ThemisGroup {
  id: string
  project_id: string
  name: string
  description: string
  tags: Record<string, string>
  created_at: string
  updated_at: string
  status: string
}

export interface ThemisRole {
  id: string
  project_id: string
  name: string
  description: string
  tags: Record<string, string>
  created_at: string
  updated_at: string
  status: string
}

export interface ThemisPolicy {
  id: string
  project_id: string
  name: string
  description: string
  document: Record<string, unknown>
  version: number
  created_at: string
  updated_at: string
  status: string
}

export interface GroupMembership {
  group_id: string
  user_id: string
  user_name: string
  created_at: string
}

export interface PolicyAttachment {
  id: string
  project_id: string
  principal_type: string
  principal_id: string
  principal_name: string
  policy_id: string
  policy_name: string
  created_at: string
}

export interface ThemisAccessKey {
  id: string
  project_id: string
  user_id: string
  user_name: string
  secret_access_key?: string
  status: string
  last_used_at: string | null
  created_at: string
  updated_at: string
}

export interface EvaluationDecision {
  allowed: boolean
  principal: string
  action: string
  resource: string
  matched_statements: string[]
}

export interface TokenResponse {
  token: string
  claims: {
    sub: string
    principal_type: string
    account: string
    project: string
    iat: string
    exp: string
  }
  expires_at: string
}

// ---- Prometheus ------------------------------------------------------------

export interface PromRuntime {
  id: string
  name: string
  image: string
  handler: string
  handler_file: string
  handler_func: string
  required_files: string[]
  description: string
}

export interface PromFunction {
  id: string
  project_id: string
  name: string
  description: string
  runtime: string
  handler: string
  timeout_ms: number
  memory_mb: number
  cpus: number
  current_version: number
  created_at: string
  updated_at: string
  status: string
}

export interface PromFunctionVersion {
  id: string
  function_id: string
  version: number
  code_sha256: string
  code_size: number
  is_active: boolean
  created_at: string
}

export interface PromInvocation {
  id: string
  function_id: string
  version: number
  status: string
  request: unknown
  response?: string
  error?: string
  exit_code?: number
  duration_ms: number
  invoked_at: string
}
