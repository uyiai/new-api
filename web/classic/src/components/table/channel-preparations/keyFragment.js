// Shared helpers for deriving the channel-name key fragment used by the
// channel preparation (备货池) add/import flows.
//
// Naming format: {timestamp}-{balance}-{suffix}-{keyFragment}
//   e.g. 202606261104-500-qq-0KVSA
//
// The key fragment is the first 5 characters of the 4th '-' separated segment
// of the key. For a standard Anthropic key:
//   sk-ant-api03-0KVSAcnC9-...
// segment[3] = "0KVSAcnC9" => fragment = "0KVSA".

// Fixed length of the key fragment.
export const KEY_FRAGMENT_LENGTH = 5;

// Index (0-based) of the key segment used for the fragment, i.e. the 4th part.
const KEY_FRAGMENT_SEGMENT_INDEX = 3;

/**
 * Extract the naming fragment from a channel key.
 *
 * Returns '' when the key has fewer than 4 segments, or the 4th segment has
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
  const segment = (segments[KEY_FRAGMENT_SEGMENT_INDEX] || '').trim();
  if (segment.length < KEY_FRAGMENT_LENGTH) return '';
  return segment.slice(0, KEY_FRAGMENT_LENGTH);
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
