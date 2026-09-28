-- name: ListSocialLinksOrdered :many
SELECT platform, display_name, handle, url, position, updated_at
FROM social_links
ORDER BY position ASC;

-- name: GetSocialLinkByPlatform :one
SELECT platform, display_name, handle, url, position, updated_at
FROM social_links
WHERE platform = ?;

-- name: UpdateSocialLink :one
UPDATE social_links
SET handle = ?, url = ?, updated_at = CURRENT_TIMESTAMP
WHERE platform = ?
RETURNING platform, display_name, handle, url, position, updated_at;
