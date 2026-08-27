import { describe, expect, it } from "vitest";

import {
  defaultScopesForCredentialType,
  scopeOptionsForCredentialType,
} from "./credentialScopes";

describe("credential scope choices", () => {
  it("preselects every permission needed by the StarLoader desktop session lifecycle", () => {
    expect(defaultScopesForCredentialType("publishable")).toEqual([
      "auth.login",
      "device.verify",
      "auth.refresh",
      "auth.logout",
    ]);
  });

  it("only presents publishable permissions for a publishable key", () => {
    expect(scopeOptionsForCredentialType("publishable").map((scope) => scope.value)).toContain("auth.login");
    expect(scopeOptionsForCredentialType("publishable").map((scope) => scope.value)).not.toContain("users.read");
  });

  it("only presents server permissions for a secret key", () => {
    expect(scopeOptionsForCredentialType("secret").map((scope) => scope.value)).toContain("users.read");
    expect(scopeOptionsForCredentialType("secret").map((scope) => scope.value)).not.toContain("auth.login");
  });
});
