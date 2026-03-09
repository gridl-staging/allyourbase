import type { Table } from "../types";
import { Key, ArrowRight } from "lucide-react";
import { cn } from "../lib/utils";

export function SchemaView({ table }: { table: Table }) {
  return (
    <div className="p-6 space-y-6 max-w-4xl">
      {/* Columns */}
      <section>
        <h2 className="text-sm font-semibold mb-3 text-gray-700">Columns</h2>
        <div className="border rounded-lg overflow-hidden">
          <table className="w-full text-sm">
            <thead className="bg-gray-50">
              <tr>
                <th className="px-4 py-2 text-left font-medium text-gray-600">
                  Name
                </th>
                <th className="px-4 py-2 text-left font-medium text-gray-600">
                  Type
                </th>
                <th className="px-4 py-2 text-left font-medium text-gray-600">
                  Nullable
                </th>
                <th className="px-4 py-2 text-left font-medium text-gray-600">
                  Default
                </th>
              </tr>
            </thead>
            <tbody>
              {table.columns.map((col) => (
                <tr key={col.name} className="border-t">
                  <td className="px-4 py-2 font-medium">
                    <span className="inline-flex items-center gap-1.5">
                      {col.isPrimaryKey && (
                        <Key className="w-3.5 h-3.5 text-amber-500" />
                      )}
                      {col.name}
                    </span>
                  </td>
                  <td className="px-4 py-2">
                    <code
                      className={cn(
                        "text-xs px-1.5 py-0.5 rounded",
                        "bg-blue-50 text-blue-700",
                      )}
                    >
                      {col.type}
                    </code>
                    {col.enumValues && col.enumValues.length > 0 && (
                      <span className="ml-2 text-xs text-gray-400">
                        [{col.enumValues.join(", ")}]
                      </span>
                    )}
                  </td>
                  <td className="px-4 py-2 text-gray-500">
                    {col.nullable ? "yes" : "no"}
                  </td>
                  <td className="px-4 py-2 text-gray-500 font-mono text-xs">
                    {col.default || "\u2014"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>

      {/* Foreign Keys */}
      {table.foreignKeys && table.foreignKeys.length > 0 && (
        <section>
          <h2 className="text-sm font-semibold mb-3 text-gray-700">
            Foreign Keys
          </h2>
          <div className="space-y-2">
            {table.foreignKeys.map((fk) => (
              <div
                key={fk.constraintName}
                className="border rounded-lg px-4 py-3 text-sm"
              >
                <div className="font-medium text-gray-700 mb-1">
                  {fk.constraintName}
                </div>
                <div className="flex items-center gap-2 text-gray-500">
                  <code className="text-xs bg-gray-100 rounded px-1.5 py-0.5">
                    {fk.columns.join(", ")}
                  </code>
                  <ArrowRight className="w-3.5 h-3.5" />
                  <code className="text-xs bg-gray-100 rounded px-1.5 py-0.5">
                    {fk.referencedSchema}.{fk.referencedTable}(
                    {fk.referencedColumns.join(", ")})
                  </code>
                </div>
                {(fk.onUpdate || fk.onDelete) && (
                  <div className="mt-1 text-xs text-gray-400">
                    {fk.onUpdate && `ON UPDATE ${fk.onUpdate}`}
                    {fk.onUpdate && fk.onDelete && " \u00B7 "}
                    {fk.onDelete && `ON DELETE ${fk.onDelete}`}
                  </div>
                )}
              </div>
            ))}
          </div>
        </section>
      )}

      {/* Indexes */}
      {table.indexes && table.indexes.length > 0 && (
        <section>
          <h2 className="text-sm font-semibold mb-3 text-gray-700">Indexes</h2>
          <div className="border rounded-lg overflow-hidden">
            <table className="w-full text-sm">
              <thead className="bg-gray-50">
                <tr>
                  <th className="px-4 py-2 text-left font-medium text-gray-600">
                    Name
                  </th>
                  <th className="px-4 py-2 text-left font-medium text-gray-600">
                    Method
                  </th>
                  <th className="px-4 py-2 text-left font-medium text-gray-600">
                    Unique
                  </th>
                  <th className="px-4 py-2 text-left font-medium text-gray-600">
                    Definition
                  </th>
                </tr>
              </thead>
              <tbody>
                {table.indexes.map((idx) => (
                  <tr key={idx.name} className="border-t">
                    <td className="px-4 py-2 font-medium">{idx.name}</td>
                    <td className="px-4 py-2 text-gray-500">{idx.method}</td>
                    <td className="px-4 py-2 text-gray-500">
                      {idx.isUnique ? "yes" : "no"}
                    </td>
                    <td className="px-4 py-2 font-mono text-xs text-gray-500">
                      {idx.definition}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      )}

      {/* Relationships */}
      {table.relationships && table.relationships.length > 0 && (
        <section>
          <h2 className="text-sm font-semibold mb-3 text-gray-700">
            Relationships
          </h2>
          <div className="space-y-2">
            {table.relationships.map((rel) => (
              <div
                key={rel.name}
                className="border rounded-lg px-4 py-3 text-sm"
              >
                <div className="flex items-center gap-2">
                  <span className="font-medium">{rel.fieldName}</span>
                  <span
                    className={cn(
                      "text-xs px-1.5 py-0.5 rounded",
                      rel.type === "many-to-one"
                        ? "bg-purple-50 text-purple-700"
                        : "bg-green-50 text-green-700",
                    )}
                  >
                    {rel.type}
                  </span>
                </div>
                <div className="text-xs text-gray-400 mt-1">
                  {rel.fromTable}({rel.fromColumns.join(", ")}) &rarr;{" "}
                  {rel.toTable}({rel.toColumns.join(", ")})
                </div>
              </div>
            ))}
          </div>
        </section>
      )}

      {/* Comment */}
      {table.comment && (
        <section>
          <h2 className="text-sm font-semibold mb-2 text-gray-700">Comment</h2>
          <p className="text-sm text-gray-600">{table.comment}</p>
        </section>
      )}
    </div>
  );
}
