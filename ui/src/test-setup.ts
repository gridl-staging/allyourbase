import "@testing-library/jest-dom/vitest";

// jsdom 28.x does not expose the full Storage interface on window.localStorage
// in the vitest 4.x environment. Provide a compliant in-memory implementation
// so tests can call localStorage.clear(), getItem(), setItem(), etc.
const makeLocalStorage = () => {
  let store: Record<string, string> = {};
  return {
    getItem: (key: string): string | null => store[key] ?? null,
    setItem: (key: string, value: string): void => {
      store[key] = String(value);
    },
    removeItem: (key: string): void => {
      delete store[key];
    },
    clear: (): void => {
      store = {};
    },
    get length(): number {
      return Object.keys(store).length;
    },
    key: (index: number): string | null => Object.keys(store)[index] ?? null,
  };
};

Object.defineProperty(window, "localStorage", {
  value: makeLocalStorage(),
  writable: true,
  configurable: true,
});
