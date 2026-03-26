#!/usr/bin/env tsx
/**
 * @module Pre-test validation script that checks if the AYB server, database, and admin credentials are properly configured before executing browser tests.
 */
/**
 * Pre-test validation script.
 *
 * Checks that all prerequisites are met before running browser tests:
 * - AYB server is running
 * - Database is connected
 * - Admin password is correct
 * - Basic API endpoints are accessible
 *
 * Run this before test execution to catch environment issues early.
 *
 * Usage:
 *   npx tsx validate-setup.ts
 *
 * Environment variables:
 *   AYB_BASE_URL - Base URL of AYB server (default: http://localhost:8090)
 *   AYB_ADMIN_PASSWORD - Admin password to test with
 */

const BASE_URL = process.env.AYB_BASE_URL || "http://localhost:8090";
const ADMIN_PASSWORD = process.env.AYB_ADMIN_PASSWORD;

interface ValidationError {
  check: string;
  message: string;
  hint?: string;
}

const errors: ValidationError[] = [];

/**
 * Validates prerequisites for running browser tests: server connectivity, admin authentication, database connectivity, and schema initialization. Logs detailed progress messages and error hints to console.
 * @returns - True if all validation checks pass; false if any errors are encountered.
 */
async function validate(): Promise<boolean> {
  console.log("🔍 Validating browser test prerequisites...\n");

  // 1. Check if admin password is set
  if (!ADMIN_PASSWORD) {
    errors.push({
      check: "Admin password",
      message: "AYB_ADMIN_PASSWORD environment variable not set",
      hint: "Set it with: export AYB_ADMIN_PASSWORD=<your-password>",
    });
  }

  // 2. Check if server is running
  console.log("✓ Checking if AYB server is running...");
  try {
    const res = await fetch(`${BASE_URL}/api/admin/status`, {
      method: "GET",
    });

    if (!res.ok) {
      errors.push({
        check: "Server connectivity",
        message: `Server responded with status ${res.status}`,
        hint: "Is AYB running? Try: ./ayb start",
      });
    } else {
      console.log(`  ✅ Server is running at ${BASE_URL}`);
    }
  } catch (err) {
    errors.push({
      check: "Server connectivity",
      message: `Cannot connect to ${BASE_URL}`,
      hint: "Is AYB running? Try: ./ayb start",
    });
  }

  // 3. Check admin authentication (if password is set)
  if (ADMIN_PASSWORD) {
    console.log("✓ Checking admin authentication...");
    try {
      const res = await fetch(`${BASE_URL}/api/admin/auth`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ password: ADMIN_PASSWORD }),
      });

      if (!res.ok) {
        const body = await res.json().catch(() => ({}));
        errors.push({
          check: "Admin authentication",
          message: `Auth failed with status ${res.status}: ${body.message || "unknown error"}`,
          hint: "Is the AYB_ADMIN_PASSWORD correct? Check with: ./ayb config get admin.password",
        });
      } else {
        const body = await res.json();
        if (!body.token) {
          errors.push({
            check: "Admin authentication",
            message: "Auth succeeded but no token in response",
            hint: "Server may be in an invalid state. Try restarting: ./ayb restart",
          });
        } else {
          console.log("  ✅ Admin authentication successful");

          // 4. Check database connectivity via a simple SQL query
          console.log("✓ Checking database connectivity...");
          try {
            const sqlRes = await fetch(`${BASE_URL}/api/admin/sql`, {
              method: "POST",
              headers: {
                "Content-Type": "application/json",
                "Authorization": `Bearer ${body.token}`,
              },
              body: JSON.stringify({ query: "SELECT 1 as test" }),
            });

            if (!sqlRes.ok) {
              const sqlBody = await sqlRes.json().catch(() => ({}));
              errors.push({
                check: "Database connectivity",
                message: `SQL query failed with status ${sqlRes.status}: ${sqlBody.message || "unknown error"}`,
                hint: "Database may not be running. Check logs: ./ayb logs",
              });
            } else {
              const sqlBody = await sqlRes.json();
              if (sqlBody.rows && sqlBody.rows.length > 0) {
                console.log("  ✅ Database is connected and responsive");
              } else {
                errors.push({
                  check: "Database connectivity",
                  message: "SQL query succeeded but returned no rows",
                  hint: "Database may be in an invalid state",
                });
              }
            }
          } catch (err) {
            errors.push({
              check: "Database connectivity",
              message: `SQL query failed: ${err}`,
              hint: "Database may not be running. Check logs: ./ayb logs",
            });
          }

          // 5. Check schema endpoint (validates database is initialized)
          console.log("✓ Checking database schema...");
          try {
            const schemaRes = await fetch(`${BASE_URL}/api/schema`, {
              method: "GET",
              headers: { "Authorization": `Bearer ${body.token}` },
            });

            if (!schemaRes.ok) {
              const schemaBody = await schemaRes.json().catch(() => ({}));
              errors.push({
                check: "Database schema",
                message: `Schema endpoint failed with status ${schemaRes.status}: ${schemaBody.message || "unknown error"}`,
                hint: "Database may not be initialized properly",
              });
            } else {
              const schemaBody = await schemaRes.json();
              if (schemaBody.tables && typeof schemaBody.tables === 'object') {
                console.log("  ✅ Database schema is accessible");
              } else {
                errors.push({
                  check: "Database schema",
                  message: "Schema endpoint returned unexpected format",
                  hint: "Database may not be initialized properly",
                });
              }
            }
          } catch (err) {
            errors.push({
              check: "Database schema",
              message: `Schema check failed: ${err}`,
              hint: "Database may not be initialized properly",
            });
          }
        }
      }
    } catch (err) {
      errors.push({
        check: "Admin authentication",
        message: `Auth request failed: ${err}`,
        hint: "Server may not be running properly",
      });
    }
  }

  // Report results
  console.log("\n" + "=".repeat(60));
  if (errors.length === 0) {
    console.log("✅ All validation checks passed!");
    console.log("\nYou can now run browser tests:");
    console.log("  npm run test:browser");
    return true;
  } else {
    console.log("❌ Validation failed with the following errors:\n");
    errors.forEach((err, i) => {
      console.log(`${i + 1}. ${err.check}:`);
      console.log(`   Error: ${err.message}`);
      if (err.hint) {
        console.log(`   Hint: ${err.hint}`);
      }
      console.log();
    });
    console.log("Fix these issues before running browser tests.");
    return false;
  }
}

// Run validation
validate().then((success) => {
  process.exit(success ? 0 : 1);
}).catch((err) => {
  console.error("Validation script crashed:", err);
  process.exit(1);
});
