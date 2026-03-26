import type { Team, TeamMemberRole, TeamMembership } from "../types/organizations";
import { TEAM_MEMBER_ROLE_VALUES } from "../types/organizations";

// ---------------------------------------------------------------------------
// Teams section (list + create + nested team detail)
// ---------------------------------------------------------------------------

export interface OrgTeamsSectionProps {
  teams: Team[];
  selectedTeamId: string | null;
  teamMembers: TeamMembership[];
  onSelectTeam: (teamId: string) => void;
  // Team creation
  newTeamName: string;
  newTeamSlug: string;
  isCreatingTeam: boolean;
  teamCreateError: string | null;
  onTeamNameChange: (value: string) => void;
  onTeamSlugChange: (value: string) => void;
  onCreateTeam: () => void;
  // Selected team editing
  teamNameDraft: string;
  teamSlugDraft: string;
  teamInfoError: string | null;
  isSavingTeamInfo: boolean;
  isDeletingTeam: boolean;
  onSelectedTeamNameChange: (value: string) => void;
  onSelectedTeamSlugChange: (value: string) => void;
  onSaveTeam: () => void;
  onDeleteTeam: () => void;
  // Team member management
  teamRoleDraftByUserId: Partial<Record<string, TeamMemberRole>>;
  teamUpdatingUserId: string | null;
  newTeamMemberUserId: string;
  newTeamMemberRole: TeamMemberRole;
  isAddingTeamMember: boolean;
  removingTeamMemberUserId: string | null;
  teamMemberActionError: string | null;
  onTeamRoleDraftChange: (userId: string, role: TeamMemberRole) => void;
  onUpdateTeamMemberRole: (userId: string) => void;
  onTeamMemberUserIdChange: (value: string) => void;
  onTeamMemberRoleChange: (value: TeamMemberRole) => void;
  onAddTeamMember: () => void;
  onRemoveTeamMember: (userId: string) => void;
}

export function OrgTeamsSection({
  teams, selectedTeamId, teamMembers, onSelectTeam,
  newTeamName, newTeamSlug, isCreatingTeam, teamCreateError,
  onTeamNameChange, onTeamSlugChange, onCreateTeam,
  teamNameDraft, teamSlugDraft, teamInfoError, isSavingTeamInfo, isDeletingTeam,
  onSelectedTeamNameChange, onSelectedTeamSlugChange, onSaveTeam, onDeleteTeam,
  teamRoleDraftByUserId, teamUpdatingUserId,
  newTeamMemberUserId, newTeamMemberRole, isAddingTeamMember,
  removingTeamMemberUserId, teamMemberActionError,
  onTeamRoleDraftChange, onUpdateTeamMemberRole,
  onTeamMemberUserIdChange, onTeamMemberRoleChange,
  onAddTeamMember, onRemoveTeamMember,
}: OrgTeamsSectionProps) {
  const selectedTeam = teams.find((t) => t.id === selectedTeamId);

  return (
    <div data-testid="org-teams-section" className="space-y-4">
      {/* Create team form */}
      <div className="rounded border border-gray-200 dark:border-gray-700 p-3 space-y-3">
        <div className="text-sm font-semibold text-gray-700 dark:text-gray-300">Create Team</div>
        <div className="flex flex-col gap-2 md:flex-row md:items-end">
          <label className="flex-1 text-sm text-gray-600 dark:text-gray-300">
            Name
            <input
              aria-label="Team Name"
              value={newTeamName}
              onChange={(e) => onTeamNameChange(e.target.value)}
              className="mt-1 w-full border rounded px-3 py-1.5 text-sm bg-white dark:bg-gray-900"
            />
          </label>
          <label className="flex-1 text-sm text-gray-600 dark:text-gray-300">
            Slug
            <input
              aria-label="Team Slug"
              value={newTeamSlug}
              onChange={(e) => onTeamSlugChange(e.target.value)}
              className="mt-1 w-full border rounded px-3 py-1.5 text-sm bg-white dark:bg-gray-900"
            />
          </label>
          <button
            onClick={onCreateTeam}
            disabled={isCreatingTeam}
            className="px-3 py-1.5 text-xs rounded bg-blue-600 text-white hover:bg-blue-700 disabled:opacity-50"
          >
            Create Team
          </button>
        </div>
        {teamCreateError && <div className="text-sm text-red-600">{teamCreateError}</div>}
      </div>

      {/* Team list */}
      {teams.length === 0 ? (
        <div className="text-sm text-gray-400 py-2">No teams found</div>
      ) : (
        <div className="space-y-1">
          {teams.map((team) => (
            <button
              key={team.id}
              onClick={() => onSelectTeam(team.id)}
              className={`w-full text-left px-3 py-2 text-sm rounded border ${
                selectedTeamId === team.id
                  ? "border-blue-300 bg-blue-50 dark:bg-blue-950 dark:border-blue-700"
                  : "border-gray-200 dark:border-gray-700 hover:bg-gray-50 dark:hover:bg-gray-900"
              }`}
            >
              <div className="font-medium text-gray-900 dark:text-gray-100">{team.name}</div>
              <div className="text-xs text-gray-500 dark:text-gray-400">{team.slug}</div>
            </button>
          ))}
        </div>
      )}

      {/* Nested team detail panel */}
      {selectedTeam && (
        <div className="rounded border border-gray-200 dark:border-gray-700 p-3 space-y-3">
          <div className="text-sm font-semibold text-gray-700 dark:text-gray-300">
            Team: {selectedTeam.name}
          </div>
          <div className="rounded border border-gray-200 dark:border-gray-700 p-3 space-y-3">
            <div className="text-sm font-semibold text-gray-700 dark:text-gray-300">Update Team</div>
            <div className="flex flex-col gap-2 md:flex-row md:items-end">
              <label className="flex-1 text-sm text-gray-600 dark:text-gray-300">
                Name
                <input
                  aria-label="Selected Team Name"
                  value={teamNameDraft}
                  onChange={(e) => onSelectedTeamNameChange(e.target.value)}
                  className="mt-1 w-full border rounded px-3 py-1.5 text-sm bg-white dark:bg-gray-900"
                />
              </label>
              <label className="flex-1 text-sm text-gray-600 dark:text-gray-300">
                Slug
                <input
                  aria-label="Selected Team Slug"
                  value={teamSlugDraft}
                  onChange={(e) => onSelectedTeamSlugChange(e.target.value)}
                  className="mt-1 w-full border rounded px-3 py-1.5 text-sm bg-white dark:bg-gray-900"
                />
              </label>
            </div>
            {teamInfoError && <div className="text-sm text-red-600">{teamInfoError}</div>}
            <div className="flex justify-end gap-2">
              <button
                onClick={onDeleteTeam}
                disabled={isDeletingTeam}
                className="px-3 py-1.5 text-xs rounded border border-red-300 text-red-700 hover:bg-red-50 disabled:opacity-50"
              >
                {isDeletingTeam ? "Deleting…" : "Delete Team"}
              </button>
              <button
                onClick={onSaveTeam}
                disabled={isSavingTeamInfo}
                className="px-3 py-1.5 text-xs rounded bg-blue-600 text-white hover:bg-blue-700 disabled:opacity-50"
              >
                {isSavingTeamInfo ? "Saving…" : "Save Team"}
              </button>
            </div>
          </div>
          <TeamMembersPanel
            members={teamMembers}
            roleDraftByUserId={teamRoleDraftByUserId}
            updatingUserId={teamUpdatingUserId}
            newMemberUserId={newTeamMemberUserId}
            newMemberRole={newTeamMemberRole}
            isAddingMember={isAddingTeamMember}
            removingUserId={removingTeamMemberUserId}
            actionError={teamMemberActionError}
            onRoleDraftChange={onTeamRoleDraftChange}
            onUpdateRole={onUpdateTeamMemberRole}
            onMemberUserIdChange={onTeamMemberUserIdChange}
            onMemberRoleChange={onTeamMemberRoleChange}
            onAddMember={onAddTeamMember}
            onRemoveMember={onRemoveTeamMember}
          />
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Team members panel (reusable within team detail)
// ---------------------------------------------------------------------------

interface TeamMembersPanelProps {
  members: TeamMembership[];
  roleDraftByUserId: Partial<Record<string, TeamMemberRole>>;
  updatingUserId: string | null;
  newMemberUserId: string;
  newMemberRole: TeamMemberRole;
  isAddingMember: boolean;
  removingUserId: string | null;
  actionError: string | null;
  onRoleDraftChange: (userId: string, role: TeamMemberRole) => void;
  onUpdateRole: (userId: string) => void;
  onMemberUserIdChange: (value: string) => void;
  onMemberRoleChange: (value: TeamMemberRole) => void;
  onAddMember: () => void;
  onRemoveMember: (userId: string) => void;
}

function TeamMembersPanel({
  members, roleDraftByUserId, updatingUserId,
  newMemberUserId, newMemberRole, isAddingMember, removingUserId, actionError,
  onRoleDraftChange, onUpdateRole,
  onMemberUserIdChange, onMemberRoleChange, onAddMember, onRemoveMember,
}: TeamMembersPanelProps) {
  return (
    <div data-testid="team-members-section" className="space-y-3">
      <div className="flex flex-col gap-2 md:flex-row md:items-end">
        <label className="flex-1 text-sm text-gray-600 dark:text-gray-300">
          User ID
          <input
            aria-label="Team Member User ID"
            value={newMemberUserId}
            onChange={(e) => onMemberUserIdChange(e.target.value)}
            className="mt-1 w-full border rounded px-3 py-1.5 text-sm bg-white dark:bg-gray-900"
          />
        </label>
        <label className="text-sm text-gray-600 dark:text-gray-300">
          Role
          <select
            aria-label="Team Member Role"
            value={newMemberRole}
            onChange={(e) => onMemberRoleChange(e.target.value as TeamMemberRole)}
            className="mt-1 w-full border rounded px-3 py-1.5 text-sm bg-white dark:bg-gray-900"
          >
            {TEAM_MEMBER_ROLE_VALUES.map((role) => (
              <option key={role} value={role}>{role}</option>
            ))}
          </select>
        </label>
        <button
          onClick={onAddMember}
          disabled={isAddingMember}
          className="px-3 py-1.5 text-xs rounded bg-blue-600 text-white hover:bg-blue-700 disabled:opacity-50"
        >
          Add Member
        </button>
      </div>
      {actionError && <div className="text-sm text-red-600">{actionError}</div>}
      {members.length === 0 ? (
        <div className="text-sm text-gray-400 py-2">No team members</div>
      ) : (
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-gray-200 dark:border-gray-700 text-left text-xs text-gray-500">
              <th className="py-2 pr-4">User ID</th>
              <th className="py-2 pr-4">Role</th>
              <th className="py-2 pr-4">Update</th>
              <th className="py-2">Remove</th>
            </tr>
          </thead>
          <tbody>
            {members.map((m) => (
              <tr key={m.id} data-testid={`team-member-row-${m.userId}`} className="border-b border-gray-100 dark:border-gray-800">
                <td className="py-1.5 pr-4 text-gray-900 dark:text-gray-100">{m.userId}</td>
                <td className="py-1.5 pr-4 text-gray-600 dark:text-gray-300">
                  <span data-testid={`team-member-role-${m.userId}`}>{m.role}</span>
                </td>
                <td className="py-1.5 pr-4">
                  <div className="flex items-center gap-2">
                    <select
                      aria-label={`Team role for ${m.userId}`}
                      value={roleDraftByUserId[m.userId] ?? m.role}
                      onChange={(e) => onRoleDraftChange(m.userId, e.target.value as TeamMemberRole)}
                      className="border rounded px-2 py-1 text-xs bg-white dark:bg-gray-900"
                    >
                      {TEAM_MEMBER_ROLE_VALUES.map((role) => (
                        <option key={role} value={role}>{role}</option>
                      ))}
                    </select>
                    <button
                      onClick={() => onUpdateRole(m.userId)}
                      disabled={updatingUserId === m.userId}
                      className="px-2 py-1 text-xs rounded border border-gray-300 text-gray-700 hover:bg-gray-100 disabled:opacity-50"
                    >
                      Update Role
                    </button>
                  </div>
                </td>
                <td className="py-1.5">
                  <button
                    onClick={() => onRemoveMember(m.userId)}
                    disabled={removingUserId === m.userId}
                    className="px-2 py-1 text-xs rounded border border-red-300 text-red-700 hover:bg-red-50 disabled:opacity-50"
                  >
                    Remove
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
