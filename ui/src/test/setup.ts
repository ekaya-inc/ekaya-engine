import { cleanup } from '@testing-library/react';
import { afterEach } from 'vitest';
import '@testing-library/jest-dom/vitest';

// Node.js v25+ defines native globalThis.localStorage and sessionStorage.
// localStorage is non-functional when --localstorage-file is not set.
// sessionStorage works but uses native C++ dispatch that vi.spyOn cannot
// intercept (sessionStorage.setItem !== Storage.prototype.setItem).
// Replace both with plain JS objects so happy-dom-style behavior and
// vi.spyOn work correctly in tests.
function createMemoryStorage(): Storage {
  let store = new Map<string, string>();
  return {
    getItem: (key: string) => store.get(key) ?? null,
    setItem: (key: string, value: string) => { store.set(key, String(value)); },
    removeItem: (key: string) => { store.delete(key); },
    clear: () => { store = new Map(); },
    get length() { return store.size; },
    key: (index: number) => [...store.keys()][index] ?? null,
  };
}

Object.defineProperty(globalThis, 'localStorage', {
  value: createMemoryStorage(),
  writable: true,
  configurable: true,
});
Object.defineProperty(globalThis, 'sessionStorage', {
  value: createMemoryStorage(),
  writable: true,
  configurable: true,
});

// Cleanup after each test
afterEach(() => {
  cleanup();
  sessionStorage.clear();
  localStorage.clear();
});

// Mock window.crypto for tests (if not already available)
if (!global.crypto) {
  (global as typeof globalThis & { crypto: Crypto }).crypto = {
    getRandomValues: <T extends ArrayBufferView | null>(array: T): T => {
      if (array && 'length' in array) {
        const uint8Array = array as unknown as Uint8Array;
        for (let i = 0; i < uint8Array.length; i++) {
          uint8Array[i] = Math.floor(Math.random() * 256);
        }
      }
      return array;
    },
    randomUUID: () => {
      return '00000000-0000-0000-0000-000000000000';
    },
    subtle: {} as SubtleCrypto,
  } as Crypto;
}

// Mock window.location for tests
delete (window as { location?: unknown }).location;
Object.defineProperty(window, 'location', {
  value: {
    href: 'http://localhost:5173',
    origin: 'http://localhost:5173',
    pathname: '/',
    search: '',
    hash: '',
    host: 'localhost:5173',
    hostname: 'localhost',
    port: '5173',
    protocol: 'http:',
    assign: () => {},
    reload: () => {},
    replace: () => {},
    ancestorOrigins: {} as DOMStringList,
  },
  writable: true,
  configurable: true,
});
