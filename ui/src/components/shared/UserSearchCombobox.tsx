import { useEffect, useId, useRef, useState } from "react";
import type { KeyboardEvent } from "react";
import { listUsers } from "../../api_admin";
import type { AdminUser } from "../../types";

const SEARCH_DEBOUNCE_MS = 300;
const SEARCH_RESULT_LIMIT = 10;

export interface UserSearchComboboxProps {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  id?: string;
  "aria-label"?: string;
}

interface UserSearchState {
  results: AdminUser[];
  loading: boolean;
  errorMessage: string | null;
  hasSearchedNoResults: boolean;
  clearSearchState: () => void;
}

type SetActiveIndex = (nextIndex: number | ((previousIndex: number) => number)) => void;
type ReopenDirection = "first" | "last";

interface ComboboxPopupState {
  activeIndex: number;
  activeOptionId?: string;
  hasPopupContent: boolean;
  visibleListbox: boolean;
  listboxId: string;
  closeListbox: () => void;
  openListbox: () => void;
  reopenListbox: (direction: ReopenDirection) => void;
  setActiveIndex: SetActiveIndex;
}

interface UserResultsListboxProps {
  listboxId: string;
  results: AdminUser[];
  loading: boolean;
  errorMessage: string | null;
  hasSearchedNoResults: boolean;
  activeIndex: number;
  onSelect: (user: AdminUser) => void;
  onActiveIndexChange: (index: number) => void;
}

interface UserSearchInputProps {
  id?: string;
  value: string;
  placeholder: string;
  ariaLabel?: string;
  visibleListbox: boolean;
  listboxId: string;
  activeOptionId?: string;
  hasPopupContent: boolean;
  containerRef: { current: HTMLDivElement | null };
  onChange: (value: string) => void;
  onOpen: () => void;
  onClose: () => void;
  onKeyDown: (event: KeyboardEvent<HTMLInputElement>) => void;
}

function formatShortUserId(userId: string): string {
  if (userId.length <= 8) return userId;
  return `${userId.slice(0, 8)}...`;
}

function shouldShowListbox(options: {
  isOpen: boolean;
  loading: boolean;
  errorMessage: string | null;
  hasSearchResult: boolean;
  hasSearchedNoResults: boolean;
}): boolean {
  if (!options.isOpen) return false;
  if (options.loading) return true;
  if (options.errorMessage) return true;
  if (options.hasSearchResult) return true;
  return options.hasSearchedNoResults;
}

function getStatusMessage(options: {
  loading: boolean;
  errorMessage: string | null;
  hasSearchedNoResults: boolean;
}): string | null {
  if (options.loading) return "Searching users...";
  if (options.errorMessage) return options.errorMessage;
  if (options.hasSearchedNoResults) return "No users found";
  return null;
}

function getActiveOptionId(options: {
  activeIndex: number;
  listboxId: string;
  resultCount: number;
}): string | undefined {
  if (options.activeIndex < 0 || options.activeIndex >= options.resultCount) {
    return undefined;
  }
  return `${options.listboxId}-option-${options.activeIndex}`;
}

function useUserSearchState(
  value: string,
  suppressSearchValueRef: { current: string | null },
): UserSearchState {
  const [results, setResults] = useState<AdminUser[]>([]);
  const [loading, setLoading] = useState(false);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [hasSearchedNoResults, setHasSearchedNoResults] = useState(false);
  const requestSequenceRef = useRef(0);

  const resetSearchState = () => {
    setResults([]);
    setLoading(false);
    setErrorMessage(null);
    setHasSearchedNoResults(false);
  };

  const beginSearch = () => {
    setResults([]);
    setLoading(true);
    setErrorMessage(null);
    setHasSearchedNoResults(false);
  };

  useEffect(() => {
    const requestId = requestSequenceRef.current + 1;
    requestSequenceRef.current = requestId;

    const trimmedValue = value.trim();
    if (!trimmedValue) {
      resetSearchState();
      return;
    }

    if (suppressSearchValueRef.current === value) {
      suppressSearchValueRef.current = null;
      setLoading(false);
      return;
    }

    beginSearch();

    const timeoutId = window.setTimeout(async () => {
      try {
        const response = await listUsers({
          search: trimmedValue,
          perPage: SEARCH_RESULT_LIMIT,
        });
        if (requestSequenceRef.current !== requestId) {
          return;
        }
        setResults(response.items);
        setHasSearchedNoResults(response.items.length === 0);
      } catch {
        if (requestSequenceRef.current !== requestId) {
          return;
        }
        setResults([]);
        setHasSearchedNoResults(false);
        setErrorMessage("Unable to load users");
      } finally {
        if (requestSequenceRef.current === requestId) {
          setLoading(false);
        }
      }
    }, SEARCH_DEBOUNCE_MS);

    return () => {
      window.clearTimeout(timeoutId);
    };
  }, [value]);

  return {
    results,
    loading,
    errorMessage,
    hasSearchedNoResults,
    clearSearchState: resetSearchState,
  };
}

function handleComboboxKeyDown(options: {
  event: KeyboardEvent<HTMLInputElement>;
  visibleListbox: boolean;
  hasPopupContent: boolean;
  results: AdminUser[];
  activeIndex: number;
  onReopen: (direction: ReopenDirection) => void;
  onActiveIndexChange: SetActiveIndex;
  onSelect: (user: AdminUser) => void;
  onClose: () => void;
}) {
  const {
    event,
    visibleListbox,
    hasPopupContent,
    results,
    activeIndex,
    onReopen,
    onActiveIndexChange,
    onSelect,
    onClose,
  } = options;

  if (event.key === "ArrowDown") {
    if (!visibleListbox) {
      if (!hasPopupContent) {
        return;
      }
      event.preventDefault();
      onReopen("first");
      return;
    }
    if (results.length === 0) {
      return;
    }
    event.preventDefault();
    onActiveIndexChange((previousIndex) =>
      previousIndex >= results.length - 1 ? 0 : previousIndex + 1,
    );
    return;
  }

  if (event.key === "ArrowUp") {
    if (!visibleListbox) {
      if (!hasPopupContent) {
        return;
      }
      event.preventDefault();
      onReopen("last");
      return;
    }
    if (results.length === 0) {
      return;
    }
    event.preventDefault();
    onActiveIndexChange((previousIndex) =>
      previousIndex <= 0 ? results.length - 1 : previousIndex - 1,
    );
    return;
  }

  if (event.key === "Enter" && activeIndex >= 0 && activeIndex < results.length) {
    event.preventDefault();
    onSelect(results[activeIndex]);
    return;
  }

  if (event.key === "Escape") {
    event.preventDefault();
    onClose();
  }
}

function useComboboxPopupState(options: {
  value: string;
  results: AdminUser[];
  loading: boolean;
  errorMessage: string | null;
  hasSearchedNoResults: boolean;
  suppressSearchValueRef: { current: string | null };
}): ComboboxPopupState {
  const [isOpen, setIsOpen] = useState(false);
  const [activeIndex, setActiveIndex] = useState(-1);
  const reopenDirectionRef = useRef<ReopenDirection | null>(null);
  const listboxId = useId();

  useEffect(() => {
    const trimmedValue = options.value.trim();
    setActiveIndex(-1);
    if (!trimmedValue) {
      setIsOpen(false);
      return;
    }
    if (options.suppressSearchValueRef.current !== options.value) {
      setIsOpen(true);
    }
  }, [options.value]);

  const hasSearchResult = options.results.length > 0;
  const hasPopupContent =
    options.loading ||
    hasSearchResult ||
    Boolean(options.errorMessage) ||
    options.hasSearchedNoResults;
  const visibleListbox = shouldShowListbox({
    isOpen,
    loading: options.loading,
    errorMessage: options.errorMessage,
    hasSearchResult,
    hasSearchedNoResults: options.hasSearchedNoResults,
  });

  useEffect(() => {
    if (!isOpen || reopenDirectionRef.current === null) {
      return;
    }

    if (options.results.length === 0) {
      reopenDirectionRef.current = null;
      return;
    }

    setActiveIndex(reopenDirectionRef.current === "first" ? 0 : options.results.length - 1);
    reopenDirectionRef.current = null;
  }, [isOpen, options.results.length]);

  const closeListbox = () => {
    setIsOpen(false);
    setActiveIndex(-1);
  };

  return {
    activeIndex,
    activeOptionId: getActiveOptionId({
      activeIndex,
      listboxId,
      resultCount: options.results.length,
    }),
    hasPopupContent,
    visibleListbox,
    listboxId,
    closeListbox,
    openListbox: () => setIsOpen(true),
    reopenListbox: (direction) => {
      reopenDirectionRef.current = direction;
      setIsOpen(true);
    },
    setActiveIndex,
  };
}

function UserResultsListbox({
  listboxId,
  results,
  loading,
  errorMessage,
  hasSearchedNoResults,
  activeIndex,
  onSelect,
  onActiveIndexChange,
}: UserResultsListboxProps) {
  const statusMessage = getStatusMessage({ loading, errorMessage, hasSearchedNoResults });
  const statusClassName = errorMessage
    ? "px-3 py-2 text-xs text-red-600"
    : "px-3 py-2 text-xs text-gray-500 dark:text-gray-400";

  return (
    <ul id={listboxId} role="listbox" className="max-h-56 overflow-y-auto py-1">
      {statusMessage ? (
        <li role="option" aria-disabled="true" aria-selected="false" className={statusClassName}>
          {statusMessage}
        </li>
      ) : (
        results.map((user, index) => {
          const optionId = `${listboxId}-option-${index}`;
          const isActive = index === activeIndex;
          return (
            <li
              key={user.id}
              id={optionId}
              role="option"
              aria-selected={isActive}
              className={[
                "cursor-pointer px-3 py-2 text-left",
                "hover:bg-gray-50 dark:hover:bg-gray-700",
                isActive ? "bg-gray-100 dark:bg-gray-700" : "",
              ]
                .filter(Boolean)
                .join(" ")}
              onMouseDown={(event) => {
                event.preventDefault();
              }}
              onMouseEnter={() => {
                onActiveIndexChange(index);
              }}
              onClick={() => {
                onSelect(user);
              }}
            >
              <div className="font-mono text-xs text-gray-800 dark:text-gray-200">{user.email}</div>
              <div className="mt-0.5 text-[10px] text-gray-500 dark:text-gray-400">
                {formatShortUserId(user.id)}
              </div>
            </li>
          );
        })
      )}
    </ul>
  );
}

function UserSearchInput({
  id,
  value,
  placeholder,
  ariaLabel,
  visibleListbox,
  listboxId,
  activeOptionId,
  hasPopupContent,
  containerRef,
  onChange,
  onOpen,
  onClose,
  onKeyDown,
}: UserSearchInputProps) {
  return (
    <input
      id={id}
      type="text"
      role="combobox"
      value={value}
      onChange={(event) => onChange(event.target.value)}
      onFocus={() => {
        if (value.trim() && hasPopupContent) {
          onOpen();
        }
      }}
      onBlur={(event) => {
        if (!containerRef.current?.contains(event.relatedTarget as Node | null)) {
          onClose();
        }
      }}
      onKeyDown={onKeyDown}
      placeholder={placeholder}
      autoComplete="off"
      spellCheck={false}
      className="w-full border rounded px-3 py-1.5 text-sm"
      aria-label={ariaLabel}
      aria-expanded={visibleListbox ? "true" : "false"}
      aria-controls={visibleListbox ? listboxId : undefined}
      aria-activedescendant={activeOptionId}
      aria-autocomplete="list"
    />
  );
}

export function UserSearchCombobox({
  value,
  onChange,
  placeholder = "Search user by email or ID",
  id,
  "aria-label": ariaLabel,
}: UserSearchComboboxProps) {
  const suppressSearchValueRef = useRef<string | null>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const { results, loading, errorMessage, hasSearchedNoResults, clearSearchState } =
    useUserSearchState(value, suppressSearchValueRef);
  const {
    activeIndex,
    activeOptionId,
    closeListbox,
    hasPopupContent,
    listboxId,
    openListbox,
    reopenListbox,
    setActiveIndex,
    visibleListbox,
  } = useComboboxPopupState({
    value,
    results,
    loading,
    errorMessage,
    hasSearchedNoResults,
    suppressSearchValueRef,
  });
  const selectUser = (user: AdminUser) => {
    suppressSearchValueRef.current = user.id;
    onChange(user.id);
    clearSearchState();
    closeListbox();
  };
  return (
    <div ref={containerRef} className="relative">
      <UserSearchInput
        id={id}
        value={value}
        placeholder={placeholder}
        ariaLabel={ariaLabel}
        visibleListbox={visibleListbox}
        listboxId={listboxId}
        activeOptionId={activeOptionId}
        hasPopupContent={hasPopupContent}
        containerRef={containerRef}
        onChange={onChange}
        onOpen={openListbox}
        onClose={closeListbox}
        onKeyDown={(event) => {
          handleComboboxKeyDown({
            event,
            visibleListbox,
            hasPopupContent,
            results,
            activeIndex,
            onReopen: reopenListbox,
            onActiveIndexChange: setActiveIndex,
            onSelect: selectUser,
            onClose: closeListbox,
          });
        }}
      />
      {visibleListbox && (
        <div className="absolute z-20 mt-1 w-full border rounded-md bg-white dark:bg-gray-800 shadow-lg overflow-hidden">
          <UserResultsListbox
            listboxId={listboxId}
            results={results}
            loading={loading}
            errorMessage={errorMessage}
            hasSearchedNoResults={hasSearchedNoResults}
            activeIndex={activeIndex}
            onSelect={selectUser}
            onActiveIndexChange={setActiveIndex}
          />
        </div>
      )}
    </div>
  );
}
