import { api } from '@/lib/api'

import type {
  SecurityProof,
  SecurityProofScope,
} from '../secure-verification/types'
import type { ApiResponse, PasskeyOptionsPayload, PasskeyStatus } from './types'

function proofHeaders(proofToken?: string): Record<string, string> | undefined {
  return proofToken ? { 'X-Security-Proof': proofToken } : undefined
}

export async function getPasskeyStatus(): Promise<ApiResponse<PasskeyStatus>> {
  const res = await api.get<ApiResponse<PasskeyStatus>>('/api/user/passkey')
  return res.data
}

export async function beginPasskeyRegistration(
  proofToken?: string
): Promise<ApiResponse<PasskeyOptionsPayload>> {
  const res = await api.post<ApiResponse<PasskeyOptionsPayload>>(
    '/api/user/passkey/register/begin',
    undefined,
    { headers: proofHeaders(proofToken) }
  )
  return res.data
}

export async function finishPasskeyRegistration(
  flowToken: string,
  payload: Record<string, unknown>,
  proofToken?: string
): Promise<ApiResponse> {
  const res = await api.post<ApiResponse>(
    '/api/user/passkey/register/finish',
    {
      flow_token: flowToken,
      credential: payload,
    },
    { headers: proofHeaders(proofToken), acceptAuthRotation: true }
  )
  return res.data
}

export async function deletePasskey(proofToken?: string): Promise<ApiResponse> {
  const res = await api.delete<ApiResponse>('/api/user/passkey', {
    headers: proofHeaders(proofToken),
    acceptAuthRotation: true,
  })
  return res.data
}

export async function beginPasskeyLogin(): Promise<
  ApiResponse<PasskeyOptionsPayload>
> {
  const res = await api.post<ApiResponse<PasskeyOptionsPayload>>(
    '/api/user/passkey/login/begin'
  )
  return res.data
}

export async function finishPasskeyLogin(
  flowToken: string,
  payload: Record<string, unknown>
): Promise<ApiResponse> {
  const res = await api.post<ApiResponse>(
    '/api/user/passkey/login/finish',
    { flow_token: flowToken, credential: payload },
    { skipAuthRefresh: true }
  )
  return res.data
}

export async function beginPasskeyVerification(
  scope: SecurityProofScope
): Promise<ApiResponse<PasskeyOptionsPayload>> {
  const res = await api.post<ApiResponse<PasskeyOptionsPayload>>(
    '/api/user/passkey/verify/begin',
    { scope }
  )
  return res.data
}

export async function finishPasskeyVerification(
  flowToken: string,
  payload: Record<string, unknown>
): Promise<ApiResponse<SecurityProof>> {
  const res = await api.post<ApiResponse<SecurityProof>>(
    '/api/user/passkey/verify/finish',
    { flow_token: flowToken, credential: payload }
  )
  return res.data
}
