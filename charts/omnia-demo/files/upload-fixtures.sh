set -eu
# Stand-in for a company's document store: mirrors the fixture documents into
# the in-cluster bucket. A real store changes a document's modified time only
# when someone edits it. This must behave the same: the ingestion connector
# sends the object's LastModified as the document's modified_at, and
# memory-api re-embeds (a paid cost) any document whose modified_at moved. So
# an unchanged fixture is never re-uploaded. `aws s3 sync` is not used: it
# re-uploads on mtime, and a ConfigMap mounted in a new pod gets a fresh mtime
# on every run, so every upgrade would bump every document.
#
# Inputs (env): BUCKET, PREFIX (ends in /), ENDPOINT; FIXTURES defaults to
# /fixtures.
FIXTURES="${FIXTURES:-/fixtures}"
s3() { aws --endpoint-url "$ENDPOINT" "$@"; }
until s3 s3 ls >/dev/null 2>&1; do sleep 2; done
s3 s3 mb "s3://$BUCKET" 2>/dev/null || true

# A mounted ConfigMap also holds every file again under ..data/ and a
# timestamped ..<date>/ directory. Those '..*' entries are never uploaded
# (the loop below skips them), and an earlier `cp --recursive` may have left
# copies in the bucket: clear them, or each upgrade would add another copy of
# every file.
s3 s3 rm "s3://$BUCKET/$PREFIX" --recursive --exclude '*' --include '..*'

# Upload a fixture only when its content differs from the stored object. A
# single-part upload's ETag is the MD5 of the content; any other ETag (or a
# missing object) counts as different and is uploaded, which errs toward
# writing rather than leaving a stale document.
for f in "$FIXTURES"/*.md; do
  [ -f "$f" ] || continue
  name="${f##*/}"
  case "$name" in ..*) continue ;; esac
  local_md5="$(md5sum "$f")"
  local_md5="${local_md5%% *}"
  etag="$(s3 s3api head-object --bucket "$BUCKET" --key "$PREFIX$name" --query ETag --output text 2>/dev/null || true)"
  etag="$(printf '%s' "$etag" | tr -d '"')"
  if [ "$etag" != "$local_md5" ]; then
    s3 s3 cp "$f" "s3://$BUCKET/$PREFIX$name"
  fi
done

# Mirror, not copy: drop stored documents whose fixture left the set, so the
# connector's manifest forgets them.
keys="$(s3 s3api list-objects-v2 --bucket "$BUCKET" --prefix "$PREFIX" --query 'Contents[].Key' --output text)"
if [ "$keys" != "None" ]; then
  for key in $keys; do
    name="${key#"$PREFIX"}"
    case "$name" in
      ..*|*/*) continue ;;
      *.md) [ -f "$FIXTURES/$name" ] || s3 s3 rm "s3://$BUCKET/$key" ;;
    esac
  done
fi
