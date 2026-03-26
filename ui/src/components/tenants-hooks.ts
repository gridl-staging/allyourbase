/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/src/components/tenants-hooks.ts.
 */
import { useCallback, useEffect, useRef, useState } from "react";
import {
  fetchBreakerState,
  fetchMaintenanceState,
  fetchTenantAudit,
  fetchTenantList,
  fetchTenantMembers,
  getTenant,
} from "../api_tenants";
import type {
  BreakerStateResponse,
  DetailTab,
  Tenant,
  TenantAuditEvent,
  TenantAuditQuery,
  TenantListQuery,
  TenantMaintenanceState,
  TenantMembership,
} from "../types/tenants";

interface ListState {
  items: Tenant[];
  page: number;
  perPage: number;
  totalItems: number;
  totalPages: number;
  isLoading: boolean;
  error: string | null;
}

export interface DetailState {
  tenant: Tenant | null;
  members: TenantMembership[];
  maintenance: TenantMaintenanceState | null;
  breaker: BreakerStateResponse | null;
  auditItems: TenantAuditEvent[];
  auditCount: number;
  isLoading: boolean;
  error: string | null;
}

const EMPTY_DETAIL_STATE: DetailState = {
  tenant: null,
  members: [],
  maintenance: null,
  breaker: null,
  auditItems: [],
  auditCount: 0,
  isLoading: false,
  error: null,
};

/**
 * TODO: Document useTenantListState.
 */
export function useTenantListState() {
  const [listQuery, setListQuery] = useState<TenantListQuery>({ page: 1, perPage: 20 });
  const [listState, setListState] = useState<ListState>({
    items: [],
    page: 1,
    perPage: 20,
    totalItems: 0,
    totalPages: 0,
    isLoading: true,
    error: null,
  });
  const [refreshKey, setRefreshKey] = useState(0);
  const activeRef = useRef(0);

  useEffect(() => {
    const id = ++activeRef.current;
    setListState((state) => ({ ...state, isLoading: true, error: null }));
    fetchTenantList(listQuery)
      .then((response) => {
        if (id !== activeRef.current) return;
        setListState({
          items: response.items,
          page: response.page,
          perPage: response.perPage,
          totalItems: response.totalItems,
          totalPages: response.totalPages,
          isLoading: false,
          error: null,
        });
      })
      .catch((error) => {
        if (id !== activeRef.current) return;
        setListState((state) => ({
          ...state,
          isLoading: false,
          error: String(error?.message ?? error),
        }));
      });
  }, [listQuery, refreshKey]);

  const nextPage = useCallback(() => {
    setListQuery((query) => ({ ...query, page: Math.min(query.page + 1, listState.totalPages) }));
  }, [listState.totalPages]);

  const prevPage = useCallback(() => {
    setListQuery((query) => ({ ...query, page: Math.max(query.page - 1, 1) }));
  }, []);

  const refreshList = useCallback(() => setRefreshKey((key) => key + 1), []);

  return { listState, listQuery, nextPage, prevPage, refreshList };
}

/**
 * TODO: Document useTenantDetailState.
 */
export function useTenantDetailState(
  selectedId: string | null,
  activeTab: DetailTab,
  auditQuery: TenantAuditQuery,
) {
  const [detail, setDetail] = useState<DetailState>(EMPTY_DETAIL_STATE);
  const activeRef = useRef(0);
  const previousSelectedIdRef = useRef<string | null>(null);

  useEffect(() => {
    if (!selectedId) {
      previousSelectedIdRef.current = null;
      setDetail(EMPTY_DETAIL_STATE);
      return;
    }

    const tenantChanged = previousSelectedIdRef.current !== selectedId;
    previousSelectedIdRef.current = selectedId;
    const id = ++activeRef.current;
    setDetail((state) =>
      tenantChanged
        ? { ...EMPTY_DETAIL_STATE, isLoading: true }
        : { ...state, isLoading: true, error: null },
    );

    const fetchDetail = async () => {
      const tenant = await getTenant(selectedId);
      if (id !== activeRef.current) return;

      let members: TenantMembership[] = [];
      let maintenance: TenantMaintenanceState | null = null;
      let breaker: BreakerStateResponse | null = null;
      let auditItems: TenantAuditEvent[] = [];
      let auditCount = 0;

      if (activeTab === "members") {
        const response = await fetchTenantMembers(selectedId);
        members = response.items;
      } else if (activeTab === "maintenance") {
        [maintenance, breaker] = await Promise.all([
          fetchMaintenanceState(selectedId),
          fetchBreakerState(selectedId),
        ]);
      } else if (activeTab === "audit") {
        const response = await fetchTenantAudit(selectedId, auditQuery);
        auditItems = response.items;
        auditCount = response.count;
      }

      if (id !== activeRef.current) return;
      setDetail({
        tenant,
        members,
        maintenance,
        breaker,
        auditItems,
        auditCount,
        isLoading: false,
        error: null,
      });
    };

    fetchDetail().catch((error) => {
      if (id !== activeRef.current) return;
      setDetail((state) => ({ ...state, isLoading: false, error: String(error?.message ?? error) }));
    });
  }, [selectedId, activeTab, auditQuery]);

  return { detail, setDetail };
}
