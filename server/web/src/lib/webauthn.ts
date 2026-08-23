type JsonRecord = Record<string, any>

function decodeBase64url(value: string): ArrayBuffer {
  const base64 = value
    .replace(/-/g, "+")
    .replace(/_/g, "/")
    .padEnd(Math.ceil(value.length / 4) * 4, "=")
  const bytes = Uint8Array.from(atob(base64), (character) => character.charCodeAt(0))
  return bytes.buffer
}

function encodeBase64url(value: ArrayBuffer | null): string | null {
  if (value === null) return null
  const bytes = new Uint8Array(value)
  let binary = ""
  for (const byte of bytes) binary += String.fromCharCode(byte)
  return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/g, "")
}

function requireWebAuthn() {
  if (!window.isSecureContext || !navigator.credentials) {
    throw new Error("Passkeys require HTTPS and a WebAuthn-capable browser")
  }
}

function creationOptions(options: JsonRecord): CredentialCreationOptions {
  const publicKey = options.publicKey || options
  return {
    publicKey: {
      ...publicKey,
      challenge: decodeBase64url(publicKey.challenge),
      user: { ...publicKey.user, id: decodeBase64url(publicKey.user.id) },
      excludeCredentials: (publicKey.excludeCredentials || []).map((credential: JsonRecord) => ({
        ...credential,
        id: decodeBase64url(credential.id),
      })),
    },
  }
}

function requestOptions(options: JsonRecord): CredentialRequestOptions {
  const publicKey = options.publicKey || options
  return {
    publicKey: {
      ...publicKey,
      challenge: decodeBase64url(publicKey.challenge),
      allowCredentials: (publicKey.allowCredentials || []).map((credential: JsonRecord) => ({
        ...credential,
        id: decodeBase64url(credential.id),
      })),
    },
  }
}

function serializeCredential(credential: PublicKeyCredential): JsonRecord {
  const response = credential.response
  const common = {
    id: credential.id,
    rawId: encodeBase64url(credential.rawId),
    type: credential.type,
    authenticatorAttachment: credential.authenticatorAttachment,
    clientExtensionResults: credential.getClientExtensionResults(),
  }

  if (response instanceof AuthenticatorAttestationResponse) {
    return {
      ...common,
      response: {
        attestationObject: encodeBase64url(response.attestationObject),
        clientDataJSON: encodeBase64url(response.clientDataJSON),
        transports: response.getTransports?.() || [],
      },
    }
  }

  const assertion = response as AuthenticatorAssertionResponse
  return {
    ...common,
    response: {
      authenticatorData: encodeBase64url(assertion.authenticatorData),
      clientDataJSON: encodeBase64url(assertion.clientDataJSON),
      signature: encodeBase64url(assertion.signature),
      userHandle: encodeBase64url(assertion.userHandle),
    },
  }
}

export async function createPasskey(options: JsonRecord): Promise<JsonRecord> {
  requireWebAuthn()
  const credential = await navigator.credentials.create(creationOptions(options))
  if (!(credential instanceof PublicKeyCredential)) throw new Error("Passkey enrollment was cancelled")
  return serializeCredential(credential)
}

export async function getPasskeyAssertion(options: JsonRecord): Promise<JsonRecord> {
  requireWebAuthn()
  const credential = await navigator.credentials.get(requestOptions(options))
  if (!(credential instanceof PublicKeyCredential)) throw new Error("Passkey verification was cancelled")
  return serializeCredential(credential)
}
