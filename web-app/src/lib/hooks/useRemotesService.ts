"use client";

import { useMemo } from "react";
import { createClient } from "@connectrpc/connect";
import { create } from "@bufbuild/protobuf";
import {
  RemoteService,
  RemoteConfigProtoSchema,
  DraftRemoteTargetSchema,
  TestRemoteConnectionRequestSchema,
  TrustRemoteHostKeyRequestSchema,
  GenerateRemoteIdentityRequestSchema,
  ListRemotesRequestSchema,
  CreateRemoteRequestSchema,
  DeleteRemoteRequestSchema,
  type RemoteConfigProto,
} from "@/gen/session/v1/remote_pb";
import { getConnectTransport } from "@/lib/api/transport";

/** A remote host, as listed/created via the Settings -> Remotes UI. */
export type RemoteConfigInfo = RemoteConfigProto;

/**
 * Inline connection coordinates for a remote that has NOT yet been saved to
 * config.json -- the Add Remote form's shape before "Test connection" /
 * "Trust and connect" have confirmed it's reachable (ssh-remote-workspaces
 * Phase 6, Epic 6.1). Mirrors the backend's DraftRemoteTarget message.
 */
export interface DraftRemoteTarget {
  name: string;
  host: string;
  user: string;
  /** 0 = default (22). */
  port: number;
}

export interface TestConnectionResult {
  success: boolean;
  hostKeyUnknown: boolean;
  fingerprint: string;
  errorMessage: string;
}

export interface TrustHostKeyResult {
  success: boolean;
  errorMessage: string;
}

export interface GeneratedIdentityInfo {
  publicKeyText: string;
  authorizedKeysLine: string;
}

export interface CreateRemoteInput {
  name: string;
  host: string;
  user: string;
  /** 0 = default (22). */
  port: number;
  basePath: string;
}

/**
 * React hook wrapping the ConnectRPC RemoteService client: the Settings ->
 * Remotes list (ListRemotes/DeleteRemote) and the Add Remote form's
 * generate-identity -> test-connection -> trust -> create-remote flow
 * (ssh-remote-workspaces Phase 6, Epic 6.1). A fresh client is constructed
 * per hook instance from the shared transport singleton (getConnectTransport),
 * mirroring useGitHubEnterpriseHosts/useApprovalRules's pattern.
 */
export function useRemotesService() {
  const client = useMemo(
    () => createClient(RemoteService, getConnectTransport()),
    [],
  );

  // The whole returned API is wrapped in useMemo (keyed on the already-stable `client`) so its
  // methods keep a stable identity across re-renders. Without this, every call to
  // useRemotesService() returned brand-new function objects, which is a footgun for any
  // caller putting one of them in a useEffect/useCallback dependency array (exactly what
  // RemotesPage.tsx's `refresh` does, depending on `listRemotes`) -- a fresh function identity
  // every render makes that effect re-fire every render, which re-triggers the state update
  // that caused the render, which changes identity again: an infinite ListRemotes fetch loop.
  // Found while writing remote-workspaces.spec.ts (ssh-remote-workspaces Phase 6 Epic 6.3):
  // the live page was issuing thousands of ListRemotes calls per second and the remotes list
  // was re-rendering continuously, which is also why row buttons kept "detaching" mid-click in
  // that spec before this fix.
  return useMemo(() => {
    const listRemotes = async (): Promise<RemoteConfigInfo[]> => {
      const resp = await client.listRemotes(
        create(ListRemotesRequestSchema, {}),
      );
      return resp.remotes;
    };

    const createRemote = async (
      input: CreateRemoteInput,
    ): Promise<RemoteConfigInfo> => {
      const resp = await client.createRemote(
        create(CreateRemoteRequestSchema, {
          name: input.name,
          host: input.host,
          user: input.user,
          port: input.port,
          basePath: input.basePath,
        }),
      );
      if (!resp.remote) {
        // Defensive: the backend always populates this on success (a failure
        // surfaces as a thrown ConnectError instead) -- this only guards
        // against a future contract drift silently producing a null remote.
        throw new Error("CreateRemote succeeded but returned no remote");
      }
      return resp.remote;
    };

    const deleteRemote = async (name: string): Promise<void> => {
      await client.deleteRemote(create(DeleteRemoteRequestSchema, { name }));
    };

    const generateRemoteIdentity = async (
      name: string,
    ): Promise<GeneratedIdentityInfo> => {
      const resp = await client.generateRemoteIdentity(
        create(GenerateRemoteIdentityRequestSchema, { name }),
      );
      return {
        publicKeyText: resp.publicKeyText,
        authorizedKeysLine: resp.authorizedKeysLine,
      };
    };

    const toDraftProto = (draft: DraftRemoteTarget) =>
      create(DraftRemoteTargetSchema, {
        name: draft.name,
        host: draft.host,
        user: draft.user,
        port: draft.port,
      });

    const testRemoteConnectionDraft = async (
      draft: DraftRemoteTarget,
    ): Promise<TestConnectionResult> => {
      const resp = await client.testRemoteConnection(
        create(TestRemoteConnectionRequestSchema, {
          draft: toDraftProto(draft),
        }),
      );
      return {
        success: resp.success,
        hostKeyUnknown: resp.hostKeyUnknown,
        fingerprint: resp.fingerprint,
        errorMessage: resp.errorMessage,
      };
    };

    const testRemoteConnectionSaved = async (
      remoteName: string,
    ): Promise<TestConnectionResult> => {
      const resp = await client.testRemoteConnection(
        create(TestRemoteConnectionRequestSchema, { remoteName }),
      );
      return {
        success: resp.success,
        hostKeyUnknown: resp.hostKeyUnknown,
        fingerprint: resp.fingerprint,
        errorMessage: resp.errorMessage,
      };
    };

    const trustRemoteHostKeyDraft = async (
      draft: DraftRemoteTarget,
      fingerprint: string,
    ): Promise<TrustHostKeyResult> => {
      const resp = await client.trustRemoteHostKey(
        create(TrustRemoteHostKeyRequestSchema, {
          draft: toDraftProto(draft),
          fingerprint,
        }),
      );
      return { success: resp.success, errorMessage: resp.errorMessage };
    };

    const trustRemoteHostKeySaved = async (
      remoteName: string,
      fingerprint: string,
    ): Promise<TrustHostKeyResult> => {
      const resp = await client.trustRemoteHostKey(
        create(TrustRemoteHostKeyRequestSchema, { remoteName, fingerprint }),
      );
      return { success: resp.success, errorMessage: resp.errorMessage };
    };

    return {
      listRemotes,
      createRemote,
      deleteRemote,
      generateRemoteIdentity,
      testRemoteConnectionDraft,
      testRemoteConnectionSaved,
      trustRemoteHostKeyDraft,
      trustRemoteHostKeySaved,
    };
  }, [client]);
}

// Re-exported so consumers (e.g. AddRemoteForm) don't need to import
// straight from the generated proto module for the plain message type.
export type { RemoteConfigProto };
export { RemoteConfigProtoSchema };
