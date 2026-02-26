# SocialMediaAPI — Endpoint Reference

> Base URL: `http://localhost:3001` (configurable via `BASE_URL` env var)

---

## Table of Contents

- [Authentication](#authentication)
  - [Register](#post-apiauthregister)
  - [Login](#post-apiauthlogin)
- [OAuth — Initiate (Protected)](#oauth--initiate-protected)
  - [Facebook](#get-apiauthfacebook)
  - [Instagram](#get-apiauthinstagram)
  - [TikTok](#get-apiauthtiktok)
  - [Twitter / X](#get-apiauthtwitter)
  - [YouTube](#get-apiauthyoutube)
- [OAuth — Callbacks (Public)](#oauth--callbacks-public)
- [OAuth — Result Pages](#oauth--result-pages)
- [Credentials (Protected)](#credentials-protected)
  - [Save Credentials](#post-apicredentials)
  - [Get Connected Platforms](#get-apicredentialsstatus)
  - [Disconnect Platform](#delete-apicredentialsdisconnect)
- [Media (Protected)](#media-protected)
  - [Upload Media](#post-apimedia)
  - [List Media](#get-apimedia)
  - [Delete Media](#delete-apimediaid)
- [Posts (Protected)](#posts-protected)
  - [Create / Publish / Schedule Post](#post-apiposts)
  - [List Posts](#get-apiposts)
  - [Get Single Post](#get-apipostsid)
- [Health](#health)
- [Static Files](#static-files)

---

## Authentication

### `POST /api/auth/register`

Register a new user account.

| Field      | Type   | Required | Description          |
|------------|--------|----------|----------------------|
| `email`    | string | Yes      | User email address   |
| `password` | string | Yes      | User password        |
| `name`     | string | Yes      | Display name         |

**Request:**

```bash
curl -X POST http://localhost:3001/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "email": "jane@example.com",
    "password": "s3cureP@ss!",
    "name": "Jane Doe"
  }'
```

**Response `201 Created`:**

```json
{
  "token": "eyJhbGciOiJIUzI1NiIs...",
  "user": {
    "id": "a1b2c3d4-...",
    "email": "jane@example.com",
    "name": "Jane Doe",
    "created_at": "2026-02-26T12:00:00Z"
  }
}
```

---

### `POST /api/auth/login`

Authenticate an existing user.

| Field      | Type   | Required | Description        |
|------------|--------|----------|--------------------|
| `email`    | string | Yes      | User email address |
| `password` | string | Yes      | User password      |

**Request:**

```bash
curl -X POST http://localhost:3001/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "jane@example.com",
    "password": "s3cureP@ss!"
  }'
```

**Response `200 OK`:**

```json
{
  "token": "eyJhbGciOiJIUzI1NiIs...",
  "user": {
    "id": "a1b2c3d4-...",
    "email": "jane@example.com",
    "name": "Jane Doe",
    "created_at": "2026-02-26T12:00:00Z"
  }
}
```

---

## OAuth — Initiate (Protected)

> All initiation endpoints require a valid JWT: `Authorization: Bearer <token>`
>
> They return an `auth_url` the client must open (redirect or popup) to start the platform's OAuth consent screen.

### `GET /api/auth/facebook`

Start Facebook OAuth flow.

| Query Param | Type | Required | Description |
|-------------|------|----------|-------------|
| *(none)*    |      |          |             |

**Request:**

```bash
curl http://localhost:3001/api/auth/facebook \
  -H "Authorization: Bearer <token>"
```

**Response `200 OK`:**

```json
{
  "auth_url": "https://www.facebook.com/v25.0/dialog/oauth?client_id=...&redirect_uri=...&state=...&scope=pages_show_list,pages_manage_posts,pages_read_engagement",
  "state": "abc123..."
}
```

---

### `GET /api/auth/instagram`

Start Instagram OAuth flow.

| Query Param     | Type   | Required | Description                                     |
|-----------------|--------|----------|-------------------------------------------------|
| `force_reauth`  | string | No       | `"true"` or `"false"` — force re-authentication |

**Request:**

```bash
curl "http://localhost:3001/api/auth/instagram?force_reauth=true" \
  -H "Authorization: Bearer <token>"
```

**Response `200 OK`:**

```json
{
  "auth_url": "https://www.instagram.com/oauth/authorize?client_id=...&redirect_uri=...&response_type=code&scope=instagram_business_basic,...&state=...&enable_fb_login=true",
  "state": "def456..."
}
```

---

### `GET /api/auth/tiktok`

Start TikTok OAuth flow (PKCE).

**Request:**

```bash
curl http://localhost:3001/api/auth/tiktok \
  -H "Authorization: Bearer <token>"
```

**Response `200 OK`:**

```json
{
  "auth_url": "https://www.tiktok.com/v2/auth/authorize/?client_key=...&redirect_uri=...&response_type=code&scope=user.info.basic,video.publish,video.upload&state=...&code_challenge=...&code_challenge_method=S256",
  "state": "ghi789..."
}
```

---

### `GET /api/auth/twitter`

Start Twitter / X OAuth 2.0 flow (PKCE).

**Request:**

```bash
curl http://localhost:3001/api/auth/twitter \
  -H "Authorization: Bearer <token>"
```

**Response `200 OK`:**

```json
{
  "auth_url": "https://twitter.com/i/oauth2/authorize?response_type=code&client_id=...&redirect_uri=...&scope=tweet.read+tweet.write+users.read+offline.access&state=...&code_challenge=...&code_challenge_method=S256",
  "state": "jkl012..."
}
```

---

### `GET /api/auth/youtube`

Start YouTube (Google) OAuth 2.0 flow.

**Request:**

```bash
curl http://localhost:3001/api/auth/youtube \
  -H "Authorization: Bearer <token>"
```

**Response `200 OK`:**

```json
{
  "auth_url": "https://accounts.google.com/o/oauth2/v2/auth?client_id=...&redirect_uri=...&response_type=code&scope=https://www.googleapis.com/auth/youtube.upload+...&state=...&access_type=offline&prompt=consent",
  "state": "mno345..."
}
```

---

## OAuth — Callbacks (Public)

These endpoints are called **by the platform**, not by the client directly. They receive the authorization `code` and `state`, exchange for tokens, save credentials, and redirect the user to a success/error page.

| Endpoint                       | Method | Query Params                          |
|--------------------------------|--------|---------------------------------------|
| `/auth/facebook/callback`      | GET    | `code`, `state`, `error`, `error_description` |
| `/auth/instagram/callback`     | GET    | `code`, `state`, `error`, `error_description` |
| `/auth/tiktok/callback`        | GET    | `code`, `state`, `error`, `error_description` |
| `/auth/twitter/callback`       | GET    | `code`, `state`, `error`, `error_description` |
| `/auth/youtube/callback`       | GET    | `code`, `state`, `error`, `error_description` |

On success the user is redirected to `/oauth/success?platform=<name>`.
On error the user is redirected to `/oauth/error?error=<type>&description=<msg>`.

---

## OAuth — Result Pages

| Endpoint         | Method | Query Params              | Description                  |
|------------------|--------|---------------------------|------------------------------|
| `/oauth/success` | GET    | `platform`                | HTML success page            |
| `/oauth/error`   | GET    | `error`, `description`    | HTML error page              |

---

## Credentials (Protected)

> All endpoints require `Authorization: Bearer <token>`.

### `POST /api/credentials`

Manually save platform credentials (useful for platforms where OAuth is handled externally, e.g. LinkedIn).

| Field              | Type   | Required | Description                                                  |
|--------------------|--------|----------|--------------------------------------------------------------|
| `platform`         | string | Yes      | `"twitter"`, `"facebook"`, `"linkedin"`, `"instagram"`, `"tiktok"`, `"youtube"` |
| `access_token`     | string | Yes      | Platform access token                                        |
| `refresh_token`    | string | No       | Refresh token (if available)                                 |
| `secret`           | string | No       | Token secret (e.g. OAuth 1.0a)                               |
| `expires_at`       | string | No       | Token expiry (RFC 3339)                                      |
| `token_type`       | string | No       | e.g. `"bearer"`                                              |
| `platform_user_id` | string | No       | User's ID on the platform                                    |
| `platform_page_id` | string | No       | Page/channel ID (Facebook pages, YouTube channels, etc.)     |

**Request:**

```bash
curl -X POST http://localhost:3001/api/credentials \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "platform": "linkedin",
    "access_token": "AQV...",
    "refresh_token": "AQX...",
    "expires_at": "2026-04-01T00:00:00Z",
    "token_type": "bearer",
    "platform_user_id": "urn:li:person:abc123"
  }'
```

**Response `200 OK`:**

```json
{
  "message": "Credentials saved successfully"
}
```

---

### `GET /api/credentials/status`

List all platforms and whether the user has connected credentials.

**Request:**

```bash
curl http://localhost:3001/api/credentials/status \
  -H "Authorization: Bearer <token>"
```

**Response `200 OK`:**

```json
{
  "user_id": "a1b2c3d4-...",
  "platforms": [
    { "platform": "twitter",   "connected": false },
    { "platform": "facebook",  "connected": true,  "created_at": "2026-02-20T10:00:00Z" },
    { "platform": "linkedin",  "connected": false },
    { "platform": "instagram", "connected": true,  "created_at": "2026-02-21T14:30:00Z" },
    { "platform": "tiktok",    "connected": false }
  ]
}
```

---

### `DELETE /api/credentials/disconnect`

Remove stored credentials for a platform.

| Field      | Type   | Required | Description                             |
|------------|--------|----------|-----------------------------------------|
| `platform` | string | Yes      | Platform name to disconnect             |

**Request:**

```bash
curl -X DELETE http://localhost:3001/api/credentials/disconnect \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "platform": "facebook"
  }'
```

**Response `200 OK`:**

```json
{
  "message": "facebook disconnected successfully"
}
```

---

## Media (Protected)

> All endpoints require `Authorization: Bearer <token>`.

### `POST /api/media`

Upload an image or video file. Max **10 MB** for images, **100 MB** for videos.

| Form Field | Type   | Required | Description                                  |
|------------|--------|----------|----------------------------------------------|
| `file`     | file   | Yes      | The file to upload (multipart/form-data)     |

**Allowed extensions:** `.jpg`, `.jpeg`, `.png`, `.gif`, `.webp`, `.mp4`

Content is also verified by magic-number detection — renamed/spoofed files are rejected.

**Request:**

```bash
curl -X POST http://localhost:3001/api/media \
  -H "Authorization: Bearer <token>" \
  -F "file=@/path/to/photo.jpg"
```

**Response `201 Created`:**

```json
{
  "media": {
    "id": "f1e2d3c4-...",
    "user_id": "a1b2c3d4-...",
    "filename": "photo.jpg",
    "path": "./uploads/a1b2c3d4-.../photo_1708948800.jpg",
    "url": "http://localhost:3001/uploads/a1b2c3d4-.../photo_1708948800.jpg",
    "type": "image",
    "size": 245760,
    "mime_type": "image/jpeg",
    "created_at": "2026-02-26T12:00:00Z"
  }
}
```

---

### `GET /api/media`

List all media uploaded by the authenticated user.

**Request:**

```bash
curl http://localhost:3001/api/media \
  -H "Authorization: Bearer <token>"
```

**Response `200 OK`:**

```json
[
  {
    "id": "f1e2d3c4-...",
    "user_id": "a1b2c3d4-...",
    "filename": "photo.jpg",
    "path": "./uploads/a1b2c3d4-.../photo_1708948800.jpg",
    "url": "http://localhost:3001/uploads/a1b2c3d4-.../photo_1708948800.jpg",
    "type": "image",
    "size": 245760,
    "mime_type": "image/jpeg",
    "created_at": "2026-02-26T12:00:00Z"
  }
]
```

---

### `DELETE /api/media/{id}`

Delete a media file. Only the owner can delete it.

| Path Param | Type   | Required | Description      |
|------------|--------|----------|------------------|
| `id`       | string | Yes      | Media UUID       |

**Request:**

```bash
curl -X DELETE http://localhost:3001/api/media/f1e2d3c4-... \
  -H "Authorization: Bearer <token>"
```

**Response `200 OK`:**

```json
{
  "message": "Media deleted successfully"
}
```

---

## Posts (Protected)

> All endpoints require `Authorization: Bearer <token>`.

### `POST /api/posts`

Create and immediately publish a post — or schedule it for later.

| Field            | Type       | Required | Description                                                                                           |
|------------------|------------|----------|-------------------------------------------------------------------------------------------------------|
| `content`        | string     | Yes      | Post text / caption                                                                                   |
| `platforms`      | string[]   | Yes      | Target platforms: `"twitter"`, `"facebook"`, `"linkedin"`, `"instagram"`, `"tiktok"`, `"youtube"`      |
| `post_type`      | string     | No       | `"normal"` (default), `"short"` (Reels/TikTok), or `"story"` (Stories)                                |
| `privacy_level`  | string     | No       | `"public"` (default), `"followers"`, `"friends"`, or `"private"`                                      |
| `is_sponsored`   | boolean    | No       | Mark post as sponsored/branded content (default `false`)                                              |
| `media_ids`      | string[]   | No       | Array of previously uploaded media UUIDs to attach                                                    |
| `scheduled_for`  | string     | No       | ISO 8601 / RFC 3339 datetime. If in the future, the post is scheduled instead of published immediately |

#### Post Type Rules

| `post_type` | Allowed Platforms                          | Media Requirement                                |
|-------------|--------------------------------------------|--------------------------------------------------|
| `normal`    | twitter, facebook, linkedin, instagram, youtube | Optional (any)                              |
| `short`     | instagram, facebook, tiktok                | At least one **video** required                  |
| `story`     | facebook, instagram                        | At least one media (image or video) required     |

> **Note:** TikTok *only* accepts `post_type: "short"`. Sending `"normal"` to TikTok returns an error.

#### Privacy Level Mapping

| `privacy_level` | Description                                    |
|------------------|------------------------------------------------|
| `public`         | Visible to everyone                            |
| `followers`      | Visible to followers only                      |
| `friends`        | Visible to mutual followers / close friends    |
| `private`        | Visible only to the creator                    |

**Example — Publish immediately to Facebook & Twitter:**

```bash
curl -X POST http://localhost:3001/api/posts \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "content": "Hello from the API!",
    "platforms": ["facebook", "twitter"]
  }'
```

**Response `201 Created`:**

```json
{
  "post_id": "b5c6d7e8-...",
  "results": [
    { "platform": "facebook",  "success": true, "message": "Published successfully", "post_id": "fb_12345" },
    { "platform": "twitter",   "success": true, "message": "Published successfully", "post_id": "tw_67890" }
  ]
}
```

**Example — Publish a short (Reel) to Instagram with media:**

```bash
curl -X POST http://localhost:3001/api/posts \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "content": "Check out this reel!",
    "platforms": ["instagram"],
    "post_type": "short",
    "media_ids": ["f1e2d3c4-..."]
  }'
```

**Example — Schedule a post for the future:**

```bash
curl -X POST http://localhost:3001/api/posts \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "content": "This will be published later!",
    "platforms": ["facebook", "linkedin"],
    "scheduled_for": "2026-03-01T15:00:00Z"
  }'
```

**Response `201 Created` (scheduled):**

```json
{
  "id": "b5c6d7e8-...",
  "user_id": "a1b2c3d4-...",
  "content": "This will be published later!",
  "post_type": "normal",
  "privacy_level": "public",
  "is_sponsored": false,
  "platforms": ["facebook", "linkedin"],
  "status": "scheduled",
  "scheduled_for": "2026-03-01T15:00:00Z",
  "created_at": "2026-02-26T12:00:00Z",
  "updated_at": "2026-02-26T12:00:00Z"
}
```

**Example — Publish a Story to Facebook & Instagram:**

```bash
curl -X POST http://localhost:3001/api/posts \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "content": "My story!",
    "platforms": ["facebook", "instagram"],
    "post_type": "story",
    "media_ids": ["f1e2d3c4-..."]
  }'
```

**Example — Private, sponsored post:**

```bash
curl -X POST http://localhost:3001/api/posts \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "content": "Exclusive sponsored content",
    "platforms": ["instagram"],
    "privacy_level": "followers",
    "is_sponsored": true,
    "media_ids": ["f1e2d3c4-..."]
  }'
```

**Partial failure response `502 Bad Gateway`:**

```json
{
  "error": "Failed to publish to one or more platforms",
  "failed_platforms": ["twitter"],
  "publish_response": {
    "post_id": "b5c6d7e8-...",
    "results": [
      { "platform": "facebook", "success": true,  "message": "Published successfully", "post_id": "fb_12345" },
      { "platform": "twitter",  "success": false, "message": "Token expired" }
    ]
  },
  "message": "Check publish_response.results for platform-specific details",
  "failed_summary": "Failed platforms: twitter"
}
```

---

### `GET /api/posts`

List all posts for the authenticated user.

**Request:**

```bash
curl http://localhost:3001/api/posts \
  -H "Authorization: Bearer <token>"
```

**Response `200 OK`:**

```json
[
  {
    "id": "b5c6d7e8-...",
    "user_id": "a1b2c3d4-...",
    "content": "Hello from the API!",
    "post_type": "normal",
    "privacy_level": "public",
    "is_sponsored": false,
    "platforms": ["facebook", "twitter"],
    "status": "published",
    "published_at": "2026-02-26T12:00:00Z",
    "created_at": "2026-02-26T12:00:00Z",
    "updated_at": "2026-02-26T12:00:00Z"
  }
]
```

---

### `GET /api/posts/{id}`

Get a specific post by ID. Only the owner can access it.

| Path Param | Type   | Required | Description      |
|------------|--------|----------|------------------|
| `id`       | string | Yes      | Post UUID        |

**Request:**

```bash
curl http://localhost:3001/api/posts/b5c6d7e8-... \
  -H "Authorization: Bearer <token>"
```

**Response `200 OK`:**

```json
{
  "id": "b5c6d7e8-...",
  "user_id": "a1b2c3d4-...",
  "content": "Hello from the API!",
  "post_type": "normal",
  "privacy_level": "public",
  "is_sponsored": false,
  "media_ids": ["f1e2d3c4-..."],
  "media": [
    {
      "id": "f1e2d3c4-...",
      "user_id": "a1b2c3d4-...",
      "filename": "photo.jpg",
      "url": "http://localhost:3001/uploads/a1b2c3d4-.../photo_1708948800.jpg",
      "type": "image",
      "size": 245760,
      "mime_type": "image/jpeg",
      "created_at": "2026-02-26T12:00:00Z"
    }
  ],
  "platforms": ["facebook", "twitter"],
  "status": "published",
  "published_at": "2026-02-26T12:00:00Z",
  "created_at": "2026-02-26T12:00:00Z",
  "updated_at": "2026-02-26T12:00:00Z"
}
```

---

## Health

### `GET /health`

Check if the server is running.

**Request:**

```bash
curl http://localhost:3001/health
```

**Response `200 OK`:**

```json
{
  "status": "healthy"
}
```

---

## Static Files

### `GET /uploads/*`

Serves uploaded media files directly from disk. No authentication required.

**Example:**

```bash
curl http://localhost:3001/uploads/a1b2c3d4-.../photo_1708948800.jpg --output photo.jpg
```

---

## Common Error Responses

All error responses follow this shape:

```json
{
  "error": "Description of the error"
}
```

| Status Code | Meaning                                                      |
|-------------|--------------------------------------------------------------|
| `400`       | Bad request — missing or invalid fields                      |
| `401`       | Unauthorized — missing or invalid JWT                        |
| `403`       | Forbidden — resource belongs to another user                 |
| `404`       | Not found — resource does not exist                          |
| `413`       | Payload too large — file exceeds size limit                  |
| `415`       | Unsupported media type — file content doesn't match allowed types |
| `429`       | Rate limited — too many requests                             |
| `502`       | Bad gateway — partial publish failure (some platforms failed) |

---

## Authentication Header

All **protected** endpoints require:

```
Authorization: Bearer <jwt_token>
```

The token is obtained from the `/api/auth/register` or `/api/auth/login` response.
