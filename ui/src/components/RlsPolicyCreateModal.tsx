import { ChevronRight } from "lucide-react";
import type { Table } from "../types";
import {
  POLICY_TEMPLATES,
  RLS_POLICY_COMMANDS,
  type PolicyTemplate,
} from "./rls-helpers";

export interface CreatePolicyFormState {
  name: string;
  command: string;
  usingExpression: string;
  withCheckExpression: string;
  isPermissive: boolean;
}

interface RlsPolicyCreateModalProps {
  isOpen: boolean;
  selectedTable: Pick<Table, "schema" | "name"> | null;
  formState: CreatePolicyFormState;
  isSubmitting: boolean;
  onClose: () => void;
  onSubmit: () => void;
  onApplyTemplate: (template: PolicyTemplate) => void;
  onNameChange: (value: string) => void;
  onCommandChange: (value: string) => void;
  onPermissiveChange: (value: boolean) => void;
  onUsingExpressionChange: (value: string) => void;
  onWithCheckExpressionChange: (value: string) => void;
}

export function RlsPolicyCreateModal({
  isOpen,
  selectedTable,
  formState,
  isSubmitting,
  onClose,
  onSubmit,
  onApplyTemplate,
  onNameChange,
  onCommandChange,
  onPermissiveChange,
  onUsingExpressionChange,
  onWithCheckExpressionChange,
}: RlsPolicyCreateModalProps) {
  if (!isOpen) {
    return null;
  }

  return (
    <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50">
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow-lg max-w-lg w-full mx-4 max-h-[90vh] overflow-y-auto">
        <div className="px-6 py-4 border-b">
          <h2 className="font-semibold">Create RLS Policy</h2>
          <p className="text-xs text-gray-400 dark:text-gray-500 mt-1">
            on {selectedTable?.schema}.{selectedTable?.name}
          </p>
        </div>

        <div className="px-6 py-4 space-y-3">
          <div>
            <label className="text-xs text-gray-500 dark:text-gray-400 block mb-1">Templates</label>
            <div className="flex flex-wrap gap-1">
              {POLICY_TEMPLATES.map((template) => (
                <button
                  key={template.name}
                  onClick={() => onApplyTemplate(template)}
                  className="px-2 py-1 text-xs bg-gray-100 dark:bg-gray-700 hover:bg-gray-200 dark:hover:bg-gray-700 rounded flex items-center gap-1"
                  title={template.description}
                >
                  <ChevronRight className="w-3 h-3" />
                  {template.name}
                </button>
              ))}
            </div>
          </div>

          <div>
            <label className="text-xs text-gray-500 dark:text-gray-400 block mb-0.5">Policy Name</label>
            <input
              aria-label="Policy name"
              type="text"
              value={formState.name}
              onChange={(event) => onNameChange(event.target.value)}
              className="w-full px-3 py-1.5 text-sm border rounded focus:outline-none focus:ring-2 focus:ring-blue-500"
              placeholder="owner_access"
            />
          </div>

          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="text-xs text-gray-500 dark:text-gray-400 block mb-0.5">Command</label>
              <select
                aria-label="Command"
                value={formState.command}
                onChange={(event) => onCommandChange(event.target.value)}
                className="w-full px-3 py-1.5 text-sm border rounded"
              >
                {RLS_POLICY_COMMANDS.map((command) => (
                  <option key={command} value={command}>
                    {command}
                  </option>
                ))}
              </select>
            </div>

            <div>
              <label className="text-xs text-gray-500 dark:text-gray-400 block mb-0.5">Type</label>
              <select
                aria-label="Permissive"
                value={formState.isPermissive ? "permissive" : "restrictive"}
                onChange={(event) => onPermissiveChange(event.target.value === "permissive")}
                className="w-full px-3 py-1.5 text-sm border rounded"
              >
                <option value="permissive">PERMISSIVE</option>
                <option value="restrictive">RESTRICTIVE</option>
              </select>
            </div>
          </div>

          <div>
            <label className="text-xs text-gray-500 dark:text-gray-400 block mb-0.5">
              USING Expression (for SELECT, UPDATE, DELETE)
            </label>
            <textarea
              aria-label="USING expression"
              value={formState.usingExpression}
              onChange={(event) => onUsingExpressionChange(event.target.value)}
              className="w-full px-3 py-1.5 text-xs font-mono border rounded resize-y h-16"
              placeholder="(user_id = current_setting('ayb.user_id', true)::uuid)"
              spellCheck={false}
            />
          </div>

          <div>
            <label className="text-xs text-gray-500 dark:text-gray-400 block mb-0.5">
              WITH CHECK Expression (for INSERT, UPDATE)
            </label>
            <textarea
              aria-label="WITH CHECK expression"
              value={formState.withCheckExpression}
              onChange={(event) => onWithCheckExpressionChange(event.target.value)}
              className="w-full px-3 py-1.5 text-xs font-mono border rounded resize-y h-16"
              placeholder="(user_id = current_setting('ayb.user_id', true)::uuid)"
              spellCheck={false}
            />
          </div>
        </div>

        <div className="px-6 py-3 border-t flex justify-end gap-2">
          <button
            onClick={onClose}
            className="px-3 py-1.5 text-sm border rounded hover:bg-gray-50 dark:hover:bg-gray-800 dark:bg-gray-800"
          >
            Cancel
          </button>
          <button
            onClick={onSubmit}
            disabled={isSubmitting || !formState.name.trim()}
            className="px-3 py-1.5 text-sm bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50"
          >
            {isSubmitting ? "Creating..." : "Create Policy"}
          </button>
        </div>
      </div>
    </div>
  );
}
