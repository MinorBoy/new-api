export type VerificationMethod = '2fa' | 'passkey'

export type SecurityProofScope =
  | 'channel.key.read'
  | 'passkey.register'
  | 'passkey.delete'

export interface SecurityProof {
  proof_token: string
  expires_at: number
  method: VerificationMethod
  scope: SecurityProofScope
}

export interface VerificationMethods {
  has2FA: boolean
  hasPasskey: boolean
  passkeySupported: boolean
}

export interface SecureVerificationState {
  method: VerificationMethod | null
  scope?: SecurityProofScope
  loading: boolean
  code: string
  title?: string
  description?: string
}

export interface UseSecureVerificationOptions {
  onSuccess?: (result: unknown, method: VerificationMethod) => void
  onError?: (error: unknown) => void
  successMessage?: string
  autoReset?: boolean
}

export interface StartVerificationOptions {
  scope: SecurityProofScope
  preferredMethod?: VerificationMethod
  title?: string
  description?: string
}
