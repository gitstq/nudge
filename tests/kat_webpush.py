#!/usr/bin/env python3
"""Independent cross-language known-answer test for nudge's Web Push
encryption. This is a clean-room re-implementation of RFC 8291 (message
encryption) and RFC 8188 (aes128gcm) in Python; it decrypts the vector
produced by the Go code and checks the plaintext. The AES-GCM primitive comes
from the `cryptography` package, everything else is written here directly so
that Go and Python share no crypto code.

Usage: python3 tests/kat_webpush.py [vector.json]
"""
import hashlib
import hmac
import json
import os
import struct
import sys

from cryptography.hazmat.primitives.asymmetric import ec
from cryptography.hazmat.primitives.ciphers.aead import AESGCM

CURVE = ec.SECP256R1()


def hkdf_extract(salt: bytes, ikm: bytes) -> bytes:
    return hmac.new(salt, ikm, hashlib.sha256).digest()


def hkdf_expand(prk: bytes, info: bytes, length: int) -> bytes:
    out, t, i = b"", b"", 1
    while len(out) < length:
        t = hmac.new(prk, t + info + bytes([i]), hashlib.sha256).digest()
        out += t
        i += 1
    return out[:length]


def decrypt(ua_private: bytes, auth_secret: bytes, body: bytes) -> bytes:
    # parse aes128gcm header
    salt = body[:16]
    rs = struct.unpack(">I", body[16:20])[0]
    assert rs >= 20, "bad record size"
    id_len = body[20]
    as_public = body[21:21 + id_len]
    sealed = body[21 + id_len:]

    ua_priv = ec.derive_private_key(int.from_bytes(ua_private, "big"), CURVE)
    as_pub = ec.EllipticCurvePublicKey.from_encoded_point(CURVE, as_public)
    ecdh_secret = ua_priv.exchange(ec.ECDH(), as_pub)
    ua_public = ua_priv.public_key().public_bytes(
        encoding=__import__("cryptography").hazmat.primitives.serialization.Encoding.X962,
        format=__import__("cryptography").hazmat.primitives.serialization.PublicFormat.UncompressedPoint,
    )

    # RFC 8291 section 3.4
    key_info = b"WebPush: info\x00" + ua_public + as_public
    prk_key = hkdf_extract(auth_secret, ecdh_secret)
    ikm = hkdf_expand(prk_key, key_info, 32)

    # RFC 8188 section 2.2
    prk = hkdf_extract(salt, ikm)
    cek = hkdf_expand(prk, b"Content-Encoding: aes128gcm\x00", 16)
    nonce = hkdf_expand(prk, b"Content-Encoding: nonce\x00", 12)

    record = AESGCM(cek).decrypt(nonce, sealed, None)
    assert record[-1] == 0x02, "missing last-record delimiter"
    return record[:-1]


def main() -> int:
    path = sys.argv[1] if len(sys.argv) > 1 else os.path.join(
        os.path.dirname(__file__), "webpush_vector.json")
    vec = json.load(open(path))
    plaintext = decrypt(
        bytes.fromhex(vec["ua_private"]),
        bytes.fromhex(vec["auth"]),
        bytes.fromhex(vec["ciphertext"]),
    )
    want = vec["plaintext"].encode()
    assert plaintext == want, f"mismatch:\n got={plaintext!r}\nwant={want!r}"
    print("KAT PASS: independent Python decryption matches Go ciphertext")
    print("plaintext:", plaintext.decode())
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
