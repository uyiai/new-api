/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import { useCallback, useEffect, useMemo, useState } from 'react';
import { API } from '../../helpers';

export const ANTHROPIC_IMPORT_PROFILE_OFFICIAL = 'anthropic_official';
export const ANTHROPIC_IMPORT_PROFILE_CLOUDFLARE =
  'cloudflare_anthropic_gateway';

export const parseAnthropicImportText = (text, profile) => {
  const entries = [];
  const errors = [];
  const columns = profile?.columns || [];

  text.split('\n').forEach((rawLine, index) => {
    const line = rawLine.trim();
    if (!line) return;

    const parts = line
      .split(/\t+|\s{2,}/)
      .map((item) => item.trim())
      .filter(Boolean);
    if (parts.length !== columns.length) {
      errors.push({
        line: index + 1,
        code: 'format',
        expectedColumns: columns.length,
      });
      return;
    }

    const balance = Number(parts[0]);
    if (!Number.isFinite(balance) || balance < 0) {
      errors.push({ line: index + 1, code: 'balance' });
      return;
    }

    const credentials = {};
    for (let columnIndex = 1; columnIndex < columns.length; columnIndex++) {
      credentials[columns[columnIndex]] = parts[columnIndex];
    }
    if (
      profile?.id === ANTHROPIC_IMPORT_PROFILE_CLOUDFLARE &&
      !/^[a-fA-F0-9]{32}$/.test(credentials.account_id || '')
    ) {
      errors.push({ line: index + 1, code: 'account_id' });
      return;
    }
    entries.push({
      balance,
      credentials,
      lineNumber: index + 1,
    });
  });

  return { entries, errors };
};

export const getAnthropicImportFormat = (profile) =>
  profile?.columns?.join('<Tab>') || '';

export const getAnthropicImportPlaceholder = (profile) => {
  if (profile?.id === ANTHROPIC_IMPORT_PROFILE_CLOUDFLARE) {
    return '100\t0123456789abcdef0123456789abcdef\tcfut_...';
  }
  return '100\tsk-ant-api03-...';
};

export const getImportCredentialPreview = (entry, profile) => {
  const value =
    profile?.id === ANTHROPIC_IMPORT_PROFILE_CLOUDFLARE
      ? entry?.credentials?.api_token
      : entry?.credentials?.api_key;
  if (!value) return '-';
  if (value.length <= 12) return `${value.slice(0, 4)}…`;
  return `${value.slice(0, 8)}…${value.slice(-4)}`;
};

export const getImportPreviewName = (
  entry,
  profile,
  nameSuffix,
  timestamp,
  tag,
) => {
  let baseName;
  if (profile?.id === ANTHROPIC_IMPORT_PROFILE_CLOUDFLARE) {
    const accountID = entry?.credentials?.account_id || '';
    baseName = `${timestamp}-${entry.balance}-Cloudflare-${accountID.slice(-8).toLowerCase()}`;
  } else {
    baseName = `${timestamp}-${entry.balance}-${nameSuffix || 'Anthropic'}`;
  }
  const normalizedTag = String(tag || '').trim();
  return normalizedTag ? `${baseName}-${normalizedTag}` : baseName;
};

export const useAnthropicImportProfiles = (visible) => {
  const [profiles, setProfiles] = useState([]);
  const [selectedProfileID, setSelectedProfileID] = useState('');
  const [drafts, setDrafts] = useState({});
  const [loading, setLoading] = useState(false);
  const [loadError, setLoadError] = useState('');

  useEffect(() => {
    if (!visible) return;
    let active = true;
    setLoading(true);
    setLoadError('');
    API.get('/api/channel/import/profiles')
      .then((response) => {
        if (!active) return;
        if (!response.data?.success) {
          throw new Error(response.data?.message || 'load profiles failed');
        }
        const data = response.data.data || {};
        const nextProfiles = Array.isArray(data.profiles) ? data.profiles : [];
        setProfiles(nextProfiles);
        const defaultID = nextProfiles.some(
          (item) => item.id === data.default_profile_id,
        )
          ? data.default_profile_id
          : nextProfiles[0]?.id || '';
        setSelectedProfileID(defaultID);
      })
      .catch((error) => {
        if (!active) return;
        setProfiles([]);
        setSelectedProfileID('');
        setLoadError(error.message || 'load profiles failed');
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [visible]);

  const selectedProfile = useMemo(
    () => profiles.find((item) => item.id === selectedProfileID) || null,
    [profiles, selectedProfileID],
  );
  const inputText = drafts[selectedProfileID] || '';
  const setInputText = useCallback(
    (value) => {
      setDrafts((current) => ({ ...current, [selectedProfileID]: value }));
    },
    [selectedProfileID],
  );
  const resetDrafts = useCallback(() => setDrafts({}), []);

  return {
    profiles,
    selectedProfile,
    selectedProfileID,
    setSelectedProfileID,
    inputText,
    setInputText,
    resetDrafts,
    loading,
    loadError,
  };
};
