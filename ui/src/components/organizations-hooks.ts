import { useCallback, useEffect, useRef, useState } from "react";
import {
  fetchOrgAudit,
  fetchOrgList,
  fetchOrgMembers,
  fetchOrgTenants,
  fetchOrgUsage,
  fetchTeamMembers,
  fetchTeams,
  getOrg,
  getTeam,
} from "../api_orgs";
import type {
  OrgAuditEvent,
  OrgAuditQuery,
  OrgDetailTab,
  OrgDetailResponse,
  OrgMembership,
  OrgUsageQuery,
  OrgUsageSummary,
  Team,
  TeamMembership,
} from "../types/organizations";
import type { Tenant } from "../types/tenants";

export interface OrgDetailState {
  org: OrgDetailResponse | null;
  members: OrgMembership[];
  teams: Team[];
  teamMembersByTeamId: Record<string, TeamMembership[]>;
  tenants: Tenant[];
  usage: OrgUsageSummary | null;
  auditItems: OrgAuditEvent[];
  auditCount: number;
  isLoading: boolean;
  error: string | null;
}

const EMPTY_DETAIL: OrgDetailState = {
  org: null,
  members: [],
  teams: [],
  teamMembersByTeamId: {},
  tenants: [],
  usage: null,
  auditItems: [],
  auditCount: 0,
  isLoading: false,
  error: null,
};

export function useOrgListState() {
  const [listState, setListState] = useState<{ items: import("../types/organizations").Organization[]; isLoading: boolean; error: string | null }>({
    items: [],
    isLoading: true,
    error: null,
  });
  const [refreshKey, setRefreshKey] = useState(0);
  const activeRef = useRef(0);

  useEffect(() => {
    const id = ++activeRef.current;
    setListState((state) => ({ ...state, isLoading: true, error: null }));
    fetchOrgList()
      .then((response) => {
        if (id !== activeRef.current) return;
        setListState({ items: response.items, isLoading: false, error: null });
      })
      .catch((error) => {
        if (id !== activeRef.current) return;
        setListState((state) => ({
          ...state,
          isLoading: false,
          error: String(error?.message ?? error),
        }));
      });
  }, [refreshKey]);

  const refreshList = useCallback(() => setRefreshKey((key) => key + 1), []);

  return { listState, refreshList };
}

export function useOrgDetailState(
  selectedId: string | null,
  activeTab: OrgDetailTab,
  auditQuery: OrgAuditQuery,
  usageQuery: OrgUsageQuery,
  selectedTeamId: string | null,
) {
  const [detail, setDetail] = useState<OrgDetailState>(EMPTY_DETAIL);
  const activeRef = useRef(0);
  const previousSelectedIdRef = useRef<string | null>(null);

  useEffect(() => {
    if (!selectedId) {
      previousSelectedIdRef.current = null;
      setDetail(EMPTY_DETAIL);
      return;
    }

    const orgChanged = previousSelectedIdRef.current !== selectedId;
    previousSelectedIdRef.current = selectedId;
    const id = ++activeRef.current;
    setDetail((state) =>
      orgChanged
        ? { ...EMPTY_DETAIL, isLoading: true }
        : { ...state, isLoading: true, error: null },
    );

    const fetchDetail = async () => {
      const org = await getOrg(selectedId);
      if (id !== activeRef.current) return;

      let members: OrgMembership[] = [];
      let teams: Team[] = [];
      let teamMembersByTeamId: Record<string, TeamMembership[]> = {};
      let tenants: Tenant[] = [];
      let usage: OrgUsageSummary | null = null;
      let auditItems: OrgAuditEvent[] = [];
      let auditCount = 0;

      if (activeTab === "members") {
        const response = await fetchOrgMembers(selectedId);
        members = response.items;
      } else if (activeTab === "teams") {
        const teamsResponse = await fetchTeams(selectedId);
        teams = teamsResponse.items;
        if (selectedTeamId) {
          const [selectedTeam, tmResponse] = await Promise.all([
            getTeam(selectedId, selectedTeamId),
            fetchTeamMembers(selectedId, selectedTeamId),
          ]);
          teams = teamsResponse.items.map((team) => (team.id === selectedTeamId ? selectedTeam : team));
          teamMembersByTeamId = { [selectedTeamId]: tmResponse.items };
        }
      } else if (activeTab === "tenants") {
        const response = await fetchOrgTenants(selectedId);
        tenants = response.items;
      } else if (activeTab === "usage") {
        usage = await fetchOrgUsage(selectedId, usageQuery);
      } else if (activeTab === "audit") {
        const response = await fetchOrgAudit(selectedId, auditQuery);
        auditItems = response.items;
        auditCount = response.count;
      }

      if (id !== activeRef.current) return;
      setDetail({
        org,
        members,
        teams,
        teamMembersByTeamId,
        tenants,
        usage,
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
  }, [selectedId, activeTab, auditQuery, usageQuery, selectedTeamId]);

  return { detail, setDetail };
}
