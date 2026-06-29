// Shared helpers for deriving the channel-name key fragment used by the
// channel preparation (备货池) add/import flows.
//
// Naming format: {timestamp}-{balance}-{suffix}-{keyFragment}
//   e.g. 202606261104-500-qq-0KVSA
//
// The key fragment is the first 5 characters of the key's random portion, i.e.
// everything after the fixed `sk-ant-api03-` prefix (the first 3 '-' segments).
// The random portion may itself contain '-', so we DO NOT truncate at the first
// '-' — the 5 characters are taken verbatim, dashes included.
//
//   sk-ant-api03-0KVSAcnC9-...        => "0KVSA"
//   sk-ant-api03-X-FFemHhdVEMbMuf...  => "X-FFe"

// Fixed length of the key fragment.
export const KEY_FRAGMENT_LENGTH = 5;

// Index (0-based) of the key segment used for the fragment, i.e. the 4th part.
const KEY_FRAGMENT_SEGMENT_INDEX = 3;

/**
 * Extract the naming fragment from a channel key.
 *
 * Takes the random portion of the key (everything after the first
 * KEY_FRAGMENT_SEGMENT_INDEX '-' segments, i.e. after `sk-ant-api03-`) and
 * returns its first KEY_FRAGMENT_LENGTH characters verbatim — including any '-'
 * the random portion contains (e.g. "X-FFe" for sk-ant-api03-X-FFem...).
 *
 * Returns '' when the key has fewer than 4 segments, or the random portion has
 * fewer than KEY_FRAGMENT_LENGTH characters, so callers can fall back to the
 * original three-segment name without a trailing separator.
 *
 * @param {string} key
 * @returns {string}
 */
export const extractKeyFragment = (key) => {
  const segments = String(key || '')
    .trim()
    .split('-');
  if (segments.length <= KEY_FRAGMENT_SEGMENT_INDEX) return '';
  // The random portion may itself contain '-' (e.g. sk-ant-api03-X-FFem...),
  // so rejoin from the 4th segment and slice verbatim instead of truncating at
  // the first '-'.
  const remainder = segments.slice(KEY_FRAGMENT_SEGMENT_INDEX).join('-');
  if (remainder.length < KEY_FRAGMENT_LENGTH) return '';
  return remainder.slice(0, KEY_FRAGMENT_LENGTH);
};

/**
 * Append the key fragment to a base channel name as a 4th segment.
 *
 * No-op when the fragment cannot be derived (returns the name unchanged) or is
 * already present at the tail (so re-saving an edited entry won't duplicate it).
 *
 * @param {string} baseName
 * @param {string} key
 * @returns {string}
 */
export const appendKeyFragment = (baseName, key) => {
  const name = String(baseName || '');
  const fragment = extractKeyFragment(key);
  if (!fragment) return name;
  if (name.endsWith(`-${fragment}`)) return name;
  return `${name}-${fragment}`;
};
