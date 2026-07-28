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

const INVITATION_STORAGE_KEYS = {
  affiliate: 'aff',
  signature: 'invite',
};

export function clearInvitationCredentials() {
  if (typeof window === 'undefined') return;

  try {
    window.sessionStorage.removeItem(INVITATION_STORAGE_KEYS.affiliate);
    window.sessionStorage.removeItem(INVITATION_STORAGE_KEYS.signature);
  } catch (error) {
    console.error('Failed to clear invitation session:', error);
  }

  try {
    window.localStorage.removeItem(INVITATION_STORAGE_KEYS.affiliate);
    window.localStorage.removeItem(INVITATION_STORAGE_KEYS.signature);
  } catch (error) {
    console.error('Failed to clear legacy invitation storage:', error);
  }
}

export function saveInvitationCredentials(aff, invite) {
  if (typeof window === 'undefined') return null;

  const credentials = {
    aff: String(aff || '').trim(),
    invite: String(invite || '').trim(),
  };
  clearInvitationCredentials();
  if (!credentials.aff) return null;

  try {
    window.sessionStorage.setItem(
      INVITATION_STORAGE_KEYS.affiliate,
      credentials.aff,
    );
    if (credentials.invite) {
      window.sessionStorage.setItem(
        INVITATION_STORAGE_KEYS.signature,
        credentials.invite,
      );
    }
    return credentials;
  } catch (error) {
    clearInvitationCredentials();
    console.error('Failed to save invitation session:', error);
    return null;
  }
}

export function getInvitationCredentials() {
  if (typeof window === 'undefined') return null;

  try {
    const aff = String(
      window.sessionStorage.getItem(INVITATION_STORAGE_KEYS.affiliate) || '',
    ).trim();
    const invite = String(
      window.sessionStorage.getItem(INVITATION_STORAGE_KEYS.signature) || '',
    ).trim();
    if (!aff) {
      clearInvitationCredentials();
      return null;
    }
    return { aff, invite };
  } catch (error) {
    clearInvitationCredentials();
    console.error('Failed to read invitation session:', error);
    return null;
  }
}

export function syncInvitationCredentialsFromSearch(search) {
  const params = new URLSearchParams(search);
  const hasInvitationQuery = params.has('aff') || params.has('invite');
  const credentials = saveInvitationCredentials(
    params.get('aff') || '',
    params.get('invite') || '',
  );

  if (hasInvitationQuery && typeof window !== 'undefined') {
    params.delete('aff');
    params.delete('invite');
    const remainingQuery = params.toString();
    const sanitizedUrl = `${window.location.pathname}${remainingQuery ? `?${remainingQuery}` : ''}${window.location.hash}`;
    window.history.replaceState(window.history.state, '', sanitizedUrl);
  }

  return credentials;
}
