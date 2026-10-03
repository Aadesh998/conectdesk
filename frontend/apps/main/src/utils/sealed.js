// Seals a payload to a one-time server key, so credentials are encrypted from
// this page to the handler that uses them, independent of TLS. Mirrors
// internal/sealed: ECDH on P-256, HKDF-SHA256 (salt: key ID, info: the scheme
// name) to an AES-256-GCM key, with the key ID and purpose as additional data.
//
// Web Crypto is only available in a secure context (HTTPS or localhost). There
// is deliberately no fallback to sending the payload in the clear.

const enc = new TextEncoder()

const toBase64 = (buf) => {
  const bytes = new Uint8Array(buf)
  let s = ''
  for (let i = 0; i < bytes.length; i++) s += String.fromCharCode(bytes[i])
  return btoa(s)
}

const fromBase64 = (b64) => Uint8Array.from(atob(b64), (c) => c.charCodeAt(0))

export const sealingSupported = () => !!window.crypto?.subtle && window.isSecureContext !== false

/**
 * Encrypts payload for the server.
 * @param {{key_id: string, public_key: string, info: string}} key from the server
 * @param {string} purpose what the payload is for; must match the server's
 * @param {object} payload JSON-serialisable
 * @returns {Promise<{key_id: string, epk: string, iv: string, ciphertext: string}>}
 */
export async function sealPayload(key, purpose, payload) {
  if (!sealingSupported()) {
    throw new Error('secure-context-required')
  }
  const subtle = window.crypto.subtle

  const serverKey = await subtle.importKey(
    'raw',
    fromBase64(key.public_key),
    { name: 'ECDH', namedCurve: 'P-256' },
    false,
    []
  )
  const ephemeral = await subtle.generateKey({ name: 'ECDH', namedCurve: 'P-256' }, false, [
    'deriveBits'
  ])
  const shared = await subtle.deriveBits({ name: 'ECDH', public: serverKey }, ephemeral.privateKey, 256)
  const hkdfKey = await subtle.importKey('raw', shared, 'HKDF', false, ['deriveKey'])
  const aesKey = await subtle.deriveKey(
    { name: 'HKDF', hash: 'SHA-256', salt: enc.encode(key.key_id), info: enc.encode(key.info) },
    hkdfKey,
    { name: 'AES-GCM', length: 256 },
    false,
    ['encrypt']
  )

  const iv = window.crypto.getRandomValues(new Uint8Array(12))
  const ciphertext = await subtle.encrypt(
    { name: 'AES-GCM', iv, additionalData: enc.encode(`${key.key_id}|${purpose}`) },
    aesKey,
    enc.encode(JSON.stringify(payload))
  )
  const epk = await subtle.exportKey('raw', ephemeral.publicKey)

  return {
    key_id: key.key_id,
    epk: toBase64(epk),
    iv: toBase64(iv),
    ciphertext: toBase64(ciphertext)
  }
}
