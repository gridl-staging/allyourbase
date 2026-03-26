import {
  CREATE_TENANT_ISOLATION_MODE_OPTIONS,
  CREATE_TENANT_PLAN_TIER_OPTIONS,
} from "../types/tenants";
import { UserSearchCombobox } from "./shared/UserSearchCombobox";

export interface CreateTenantFormValues {
  name: string;
  slug: string;
  ownerUserId: string;
  isolationMode: string;
  planTier: string;
  region: string;
}

export type CreateTenantFormErrors = Partial<Record<keyof CreateTenantFormValues, string>>;

interface TenantCreateDialogProps {
  isOpen: boolean;
  values: CreateTenantFormValues;
  errors: CreateTenantFormErrors;
  submitError: string | null;
  isSubmitting: boolean;
  onChange: (field: keyof CreateTenantFormValues, value: string) => void;
  onClose: () => void;
  onSubmit: () => void;
}

export function TenantCreateDialog({
  isOpen,
  values,
  errors,
  submitError,
  isSubmitting,
  onChange,
  onClose,
  onSubmit,
}: TenantCreateDialogProps) {
  if (!isOpen) {
    return null;
  }

  return (
    <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-40">
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow-xl p-6 max-w-lg w-full mx-4">
        <h3 className="font-semibold mb-4">Create Tenant</h3>
        <div className="space-y-3">
          <div>
            <label htmlFor="tenant-create-name" className="block text-sm text-gray-700 dark:text-gray-200 mb-1">
              Tenant Name
            </label>
            <input
              id="tenant-create-name"
              aria-label="Tenant Name"
              value={values.name}
              onChange={(event) => onChange("name", event.target.value)}
              className="w-full border rounded px-3 py-1.5 text-sm"
            />
            {errors.name && <p className="mt-1 text-xs text-red-600">{errors.name}</p>}
          </div>
          <div>
            <label htmlFor="tenant-create-slug" className="block text-sm text-gray-700 dark:text-gray-200 mb-1">
              Slug
            </label>
            <input
              id="tenant-create-slug"
              aria-label="Slug"
              value={values.slug}
              onChange={(event) => onChange("slug", event.target.value)}
              className="w-full border rounded px-3 py-1.5 text-sm"
            />
            {errors.slug && <p className="mt-1 text-xs text-red-600">{errors.slug}</p>}
          </div>
          <div>
            <label
              htmlFor="tenant-create-owner-user-id"
              className="block text-sm text-gray-700 dark:text-gray-200 mb-1"
            >
              Owner User ID (optional)
            </label>
            <UserSearchCombobox
              id="tenant-create-owner-user-id"
              aria-label="Owner User ID"
              value={values.ownerUserId}
              onChange={(value) => onChange("ownerUserId", value)}
              placeholder="Search by email or paste a user ID"
            />
            {errors.ownerUserId && <p className="mt-1 text-xs text-red-600">{errors.ownerUserId}</p>}
          </div>
          <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
            <div>
              <label
                htmlFor="tenant-create-isolation-mode"
                className="block text-sm text-gray-700 dark:text-gray-200 mb-1"
              >
                Isolation Mode
              </label>
              <select
                id="tenant-create-isolation-mode"
                aria-label="Isolation Mode"
                value={values.isolationMode}
                onChange={(event) => onChange("isolationMode", event.target.value)}
                className="w-full border rounded px-3 py-1.5 text-sm"
              >
                {CREATE_TENANT_ISOLATION_MODE_OPTIONS.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </select>
            </div>
            <div>
              <label
                htmlFor="tenant-create-plan-tier"
                className="block text-sm text-gray-700 dark:text-gray-200 mb-1"
              >
                Plan Tier
              </label>
              <select
                id="tenant-create-plan-tier"
                aria-label="Plan Tier"
                value={values.planTier}
                onChange={(event) => onChange("planTier", event.target.value)}
                className="w-full border rounded px-3 py-1.5 text-sm"
              >
                {CREATE_TENANT_PLAN_TIER_OPTIONS.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </select>
            </div>
            <div>
              <label htmlFor="tenant-create-region" className="block text-sm text-gray-700 dark:text-gray-200 mb-1">
                Region
              </label>
              <input
                id="tenant-create-region"
                aria-label="Region"
                value={values.region}
                onChange={(event) => onChange("region", event.target.value)}
                className="w-full border rounded px-3 py-1.5 text-sm"
                placeholder="e.g. us-east-1"
              />
              <p className="mt-1 text-xs text-gray-500 dark:text-gray-400">Region is free text.</p>
            </div>
          </div>
          {submitError && <p className="text-sm text-red-600">{submitError}</p>}
        </div>
        <div className="mt-5 flex justify-end gap-2">
          <button
            onClick={onClose}
            disabled={isSubmitting}
            className="px-3 py-1.5 text-sm rounded border border-gray-300 text-gray-700 hover:bg-gray-100 disabled:opacity-50"
          >
            Cancel
          </button>
          <button
            onClick={onSubmit}
            disabled={isSubmitting}
            className="px-3 py-1.5 text-sm rounded bg-blue-600 text-white hover:bg-blue-700 disabled:opacity-50"
          >
            Create
          </button>
        </div>
      </div>
    </div>
  );
}
