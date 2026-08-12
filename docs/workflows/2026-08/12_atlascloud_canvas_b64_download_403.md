# AtlasCloud Canvas b64 download 403

## Background

Infinite Canvas opens as an external app and calls NewAPI through the
`/canvas/v1` base URL. Image generation uses `POST /canvas/v1/images/tasks`;
NewAPI creates a `canvas_image` task and runs the real image relay in the
background.

For AtlasCloud image outputs, the adapter converts AtlasCloud prediction
outputs into OpenAI image response data. When the request asks for
`response_format=b64_json`, the adapter must download the returned image URL and
encode it as base64 before the async task can be marked successful.

## Issue

Production task `task_7lbE9wEIAv47McSAfpVxI0RuCpgaSBKT` failed before any
`tasks.data` result was stored. The failure was recorded on
`/canvas/v1/images/generations` with:

```text
atlascloud: download image failed: failed to download image: HTTP 403
```

The evidence points to NewAPI downloading an AtlasCloud output image URL for
URL-to-base64 conversion, not Canvas directly receiving an AtlasCloud image URL.

Follow-up Canvas testing also exposed a separate AtlasCloud request conversion
issue: Canvas injects `group` into the forwarded request body so NewAPI can
select the billing/routing group, but AtlasCloud rejects unknown upstream
payload fields with `Unknown parameter: 'group'`.

## Change

- Added `service.DoDownloadRequestWithHeaders` while keeping
  `DoDownloadRequest` behavior unchanged.
- Reused the existing image response validation and base64 conversion through
  `service.ImageResponseToBase64`.
- AtlasCloud `b64_json` output conversion now uses a media download helper that
  sends browser-like `Accept` and `User-Agent` headers.
- For `atlascloud.ai` and subdomains only, the helper also sends the AtlasCloud
  bearer token and `Referer: https://www.atlascloud.ai/`.
- AtlasCloud request conversion now filters the internal `group` field from
  both `extra_fields` and unknown JSON fields before sending the payload
  upstream.
- AtlasCloud prediction polling now treats HTTP 429 from the polling endpoint as
  a temporary rate limit: it honors `Retry-After` or `retry after N seconds`
  hints, waits within the existing poll timeout, and then fetches the prediction
  again.

## Safety

The AtlasCloud bearer token is only attached when the output URL hostname is
`atlascloud.ai` or ends with `.atlascloud.ai`; third-party media URLs keep the
generic unauthenticated download behavior.

SSRF validation is still performed by the existing download path.

## Verification

Run:

```powershell
go test ./service ./relay/channel/atlascloud ./controller
```
