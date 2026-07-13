#!/usr/bin/env python3
"""Minimal AWS SigV4 probe for an S3-compatible endpoint.

The script never prints credentials, endpoint values, response bodies, or headers.
It emits a single sanitized JSON object for the surrounding smoke-test script.
"""

from __future__ import annotations

import datetime as dt
import hashlib
import hmac
import json
import os
import sys
import urllib.error
import urllib.parse
import urllib.request
from dataclasses import dataclass


@dataclass(frozen=True)
class ProbeResult:
    status: int | None
    network_error: bool = False


def _signing_key(secret: str, date_stamp: str, region: str) -> bytes:
    key_date = hmac.new(("AWS4" + secret).encode(), date_stamp.encode(), hashlib.sha256).digest()
    key_region = hmac.new(key_date, region.encode(), hashlib.sha256).digest()
    key_service = hmac.new(key_region, b"s3", hashlib.sha256).digest()
    return hmac.new(key_service, b"aws4_request", hashlib.sha256).digest()


def _probe(method: str, url: str, region: str, access_key: str, secret_key: str) -> ProbeResult:
    parsed = urllib.parse.urlsplit(url)
    if parsed.scheme != "https" or not parsed.netloc:
        return ProbeResult(status=None, network_error=True)

    now = dt.datetime.now(dt.timezone.utc)
    amz_date = now.strftime("%Y%m%dT%H%M%SZ")
    date_stamp = now.strftime("%Y%m%d")
    payload_hash = hashlib.sha256(b"").hexdigest()

    canonical_uri = urllib.parse.quote(parsed.path or "/", safe="/-_.~")
    canonical_query = parsed.query
    canonical_headers = (
        f"host:{parsed.netloc}\n"
        f"x-amz-content-sha256:{payload_hash}\n"
        f"x-amz-date:{amz_date}\n"
    )
    signed_headers = "host;x-amz-content-sha256;x-amz-date"
    canonical_request = "\n".join(
        [
            method,
            canonical_uri,
            canonical_query,
            canonical_headers,
            signed_headers,
            payload_hash,
        ]
    )

    credential_scope = f"{date_stamp}/{region}/s3/aws4_request"
    string_to_sign = "\n".join(
        [
            "AWS4-HMAC-SHA256",
            amz_date,
            credential_scope,
            hashlib.sha256(canonical_request.encode()).hexdigest(),
        ]
    )
    signature = hmac.new(
        _signing_key(secret_key, date_stamp, region),
        string_to_sign.encode(),
        hashlib.sha256,
    ).hexdigest()

    authorization = (
        "AWS4-HMAC-SHA256 "
        f"Credential={access_key}/{credential_scope}, "
        f"SignedHeaders={signed_headers}, Signature={signature}"
    )
    request = urllib.request.Request(
        url,
        method=method,
        headers={
            "Authorization": authorization,
            "Host": parsed.netloc,
            "X-Amz-Content-Sha256": payload_hash,
            "X-Amz-Date": amz_date,
        },
    )

    try:
        with urllib.request.urlopen(request, timeout=20) as response:
            return ProbeResult(status=response.status)
    except urllib.error.HTTPError as exc:
        return ProbeResult(status=exc.code)
    except (urllib.error.URLError, TimeoutError, OSError):
        return ProbeResult(status=None, network_error=True)


def main() -> int:
    endpoint = os.environ.get("OBJECT_STORAGE_ENDPOINT", "").rstrip("/")
    region = os.environ.get("OBJECT_STORAGE_REGION", "")
    access_key = os.environ.get("OBJECT_STORAGE_ACCESS_KEY_ID", "")
    secret_key = os.environ.get("OBJECT_STORAGE_SECRET_ACCESS_KEY", "")
    bucket = os.environ.get("EXPECTED_STORAGE_BUCKET", "sonata-private")

    if not all((endpoint, region, access_key, secret_key, bucket)):
        print(json.dumps({"credentials": "missing", "bucket": "unknown"}))
        return 0

    list_result = _probe("GET", endpoint + "/", region, access_key, secret_key)
    bucket_path = urllib.parse.quote(bucket, safe="-_.~")
    bucket_result = _probe("HEAD", endpoint + "/" + bucket_path, region, access_key, secret_key)

    successful = {200, 204}
    auth_rejected = {401, 403}

    if list_result.status in successful or bucket_result.status in successful:
        credential_state = "ok"
    elif list_result.network_error or bucket_result.network_error:
        credential_state = "endpoint"
    elif list_result.status in auth_rejected or bucket_result.status in auth_rejected:
        credential_state = "auth"
    else:
        credential_state = "error"

    if bucket_result.status in successful:
        bucket_state = "ok"
    elif bucket_result.status == 404:
        bucket_state = "missing"
    elif bucket_result.status in auth_rejected:
        bucket_state = "inaccessible"
    elif bucket_result.network_error:
        bucket_state = "endpoint"
    else:
        bucket_state = "unknown"

    print(
        json.dumps(
            {
                "credentials": credential_state,
                "bucket": bucket_state,
                "list_http": list_result.status,
                "bucket_http": bucket_result.status,
            },
            separators=(",", ":"),
        )
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
