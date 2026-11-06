(self["webpackChunk_N_E"] = self["webpackChunk_N_E"] || []).push([[1150],{

/***/ 3728:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   PageViewTracker: () => (/* binding */ PageViewTracker)
/* harmony export */ });
/* harmony import */ var _lib_analytics_usePageView__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(90814);
/* __next_internal_client_entry_do_not_use__ PageViewTracker auto */ 
function PageViewTracker() {
    (0,_lib_analytics_usePageView__WEBPACK_IMPORTED_MODULE_0__/* .usePageView */ .O)();
    return null;
}


/***/ }),

/***/ 16446:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   I: () => (/* binding */ useRemotesService)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(66144);
/* harmony import */ var _bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(95444);
/* harmony import */ var _gen_session_v1_remote_pb__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(88149);
/* harmony import */ var _lib_api_transport__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(21694);
/* __next_internal_client_entry_do_not_use__ useRemotesService,RemoteConfigProtoSchema auto */ 




/**
 * React hook wrapping the ConnectRPC RemoteService client: the Settings ->
 * Remotes list (ListRemotes/DeleteRemote) and the Add Remote form's
 * generate-identity -> test-connection -> trust -> create-remote flow
 * (ssh-remote-workspaces Phase 6, Epic 6.1). A fresh client is constructed
 * per hook instance from the shared transport singleton (getConnectTransport),
 * mirroring useGitHubEnterpriseHosts/useApprovalRules's pattern.
 */ function useRemotesService() {
    const client = (0,react__WEBPACK_IMPORTED_MODULE_0__.useMemo)(()=>(0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_3__/* .createClient */ .UU)(_gen_session_v1_remote_pb__WEBPACK_IMPORTED_MODULE_1__/* .RemoteService */ .ef, (0,_lib_api_transport__WEBPACK_IMPORTED_MODULE_2__/* .getConnectTransport */ .qW)()), []);
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
    return (0,react__WEBPACK_IMPORTED_MODULE_0__.useMemo)(()=>{
        const listRemotes = async ()=>{
            const resp = await client.listRemotes((0,_bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_4__/* .create */ .v)(_gen_session_v1_remote_pb__WEBPACK_IMPORTED_MODULE_1__/* .ListRemotesRequestSchema */ .Cj, {}));
            return resp.remotes;
        };
        const createRemote = async (input)=>{
            const resp = await client.createRemote((0,_bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_4__/* .create */ .v)(_gen_session_v1_remote_pb__WEBPACK_IMPORTED_MODULE_1__/* .CreateRemoteRequestSchema */ .TW, {
                name: input.name,
                host: input.host,
                user: input.user,
                port: input.port,
                basePath: input.basePath
            }));
            if (!resp.remote) {
                // Defensive: the backend always populates this on success (a failure
                // surfaces as a thrown ConnectError instead) -- this only guards
                // against a future contract drift silently producing a null remote.
                throw new Error("CreateRemote succeeded but returned no remote");
            }
            return resp.remote;
        };
        const deleteRemote = async (name)=>{
            await client.deleteRemote((0,_bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_4__/* .create */ .v)(_gen_session_v1_remote_pb__WEBPACK_IMPORTED_MODULE_1__/* .DeleteRemoteRequestSchema */ .eB, {
                name
            }));
        };
        const generateRemoteIdentity = async (name)=>{
            const resp = await client.generateRemoteIdentity((0,_bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_4__/* .create */ .v)(_gen_session_v1_remote_pb__WEBPACK_IMPORTED_MODULE_1__/* .GenerateRemoteIdentityRequestSchema */ .cj, {
                name
            }));
            return {
                publicKeyText: resp.publicKeyText,
                authorizedKeysLine: resp.authorizedKeysLine
            };
        };
        const toDraftProto = (draft)=>(0,_bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_4__/* .create */ .v)(_gen_session_v1_remote_pb__WEBPACK_IMPORTED_MODULE_1__/* .DraftRemoteTargetSchema */ .ag, {
                name: draft.name,
                host: draft.host,
                user: draft.user,
                port: draft.port
            });
        const testRemoteConnectionDraft = async (draft)=>{
            const resp = await client.testRemoteConnection((0,_bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_4__/* .create */ .v)(_gen_session_v1_remote_pb__WEBPACK_IMPORTED_MODULE_1__/* .TestRemoteConnectionRequestSchema */ .Jt, {
                draft: toDraftProto(draft)
            }));
            return {
                success: resp.success,
                hostKeyUnknown: resp.hostKeyUnknown,
                fingerprint: resp.fingerprint,
                errorMessage: resp.errorMessage
            };
        };
        const testRemoteConnectionSaved = async (remoteName)=>{
            const resp = await client.testRemoteConnection((0,_bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_4__/* .create */ .v)(_gen_session_v1_remote_pb__WEBPACK_IMPORTED_MODULE_1__/* .TestRemoteConnectionRequestSchema */ .Jt, {
                remoteName
            }));
            return {
                success: resp.success,
                hostKeyUnknown: resp.hostKeyUnknown,
                fingerprint: resp.fingerprint,
                errorMessage: resp.errorMessage
            };
        };
        const trustRemoteHostKeyDraft = async (draft, fingerprint)=>{
            const resp = await client.trustRemoteHostKey((0,_bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_4__/* .create */ .v)(_gen_session_v1_remote_pb__WEBPACK_IMPORTED_MODULE_1__/* .TrustRemoteHostKeyRequestSchema */ .Ek, {
                draft: toDraftProto(draft),
                fingerprint
            }));
            return {
                success: resp.success,
                errorMessage: resp.errorMessage
            };
        };
        const trustRemoteHostKeySaved = async (remoteName, fingerprint)=>{
            const resp = await client.trustRemoteHostKey((0,_bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_4__/* .create */ .v)(_gen_session_v1_remote_pb__WEBPACK_IMPORTED_MODULE_1__/* .TrustRemoteHostKeyRequestSchema */ .Ek, {
                remoteName,
                fingerprint
            }));
            return {
                success: resp.success,
                errorMessage: resp.errorMessage
            };
        };
        return {
            listRemotes,
            createRemote,
            deleteRemote,
            generateRemoteIdentity,
            testRemoteConnectionDraft,
            testRemoteConnectionSaved,
            trustRemoteHostKeyDraft,
            trustRemoteHostKeySaved
        };
    }, [
        client
    ]);
}



/***/ }),

/***/ 18464:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 18678:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
// ESM COMPAT FLAG
__webpack_require__.r(__webpack_exports__);

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  "default": () => (/* binding */ RemotesPage)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/connect-error.js
var connect_error = __webpack_require__(45640);
// EXTERNAL MODULE: ./src/lib/hooks/useRemotesService.ts
var useRemotesService = __webpack_require__(16446);
// EXTERNAL MODULE: ./src/components/analytics/PageViewTracker.tsx
var PageViewTracker = __webpack_require__(3728);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react-dom/index.js
var react_dom = __webpack_require__(80883);
// EXTERNAL MODULE: ./src/lib/hooks/useDialogFocusTrap.ts
var useDialogFocusTrap = __webpack_require__(82279);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsettings%2FHostKeyTrustDialog.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA91Wy3LjIBC8-yvmslX2gZSkOI5DLvsnKSxGMgkCBZCteMv_viUk23rG2qqc9oQEDT3T0zwe3sKAfW4sC-DPAiDXVjihFYVElMhfFwBCWXQUgup7x-KP1OhCcRJrqQ2FAzNLQt7CFzxF5sRWFYoLm0v2RSGRWFYdTIpUEeEwsxRiVA5N1f1eWCeSLxJr5VC59tCJCMWxpBAGgWfOGedCpX3Cx3D1ujgvHi5ZhD6Le2G--DB32nA0FMK8BKul4D3UewtFDOOisAP6bYMpid0zro8DQLb6LvrIj2asJEfB3Z7CehvkXrLmP2YyXoZB8AsIRAazcX2rlnBhMK5rF2tZZKoaSVl-T7LIS5Zo5YgVJxzAuef040cU6d4NEHuPGFU6aBI0qVDeQy3iR088Oi28cY7GFPeXBZBCIdk3AYYPTx2qtae66pYa4a1dtcRhlkvmkNSqWQqscBrCxDRZFZkiYzrWuRl9HBuNTsMQp1xQL3QzbR8Qz7IrzrDrc7f0T3dKv7tb-nS69NE3pd_MIp7nKICjNpzsDLIPCr4hTMoO3_ONL2GZkF-DYIuuMtuuYS4bbdoGg7PM70lU_BYtcXrcJi3el1nnVzYtz2fHK0ornOeKSXdGp9XYAXJ_d86wTWFslUGuxeXcd4apyw3Um6Pqwgt13eXrdXVWttWje31AM0tD1ROeJjouLDkIK3YS_RK6cNWpQiGa2HJ1HRoY0Unib8qoFxWbiMfnmjODyk2WM_zRrf9fFJn9Q5G3bWFGEWLVW_ynbfA7Qy4YLFuX_CYI8nLl1-49XEYfYAASk8sLDMDUujR_O-2czq6_LZbq1VB3DjoaQeq5pFr96ppgDOEpu5BzO_htE_z4O4QYPKCxWE87_wUA5jxGcgoAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsettings_2FHostKeyTrustDialog_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA91Wy3LjIBC8_yvmslX2gZSkOI5DLvsnKSxGMgkCBZCteMv_viUk23rG2qqc9oQEDT3T0zwe3sKAfW4sC_DPAiDXVjihFYVElMhfFwBCWXQUgup7x_KP1OhCcRJrqQ2FAzNLQt7CFzxF5sRWFYoLm0v2RSGRWFYdTIpUEeEwsxRiVA5N1f1eWCeSLxJr5VC59tCJCMWxpBAGgWfOGedCpX3Cx3D1ujgvHi5ZhD6Le2G_DB32nA0FMK8BKul4D3UewtFDOOisAP6bYMpid0zro8DQLb6LvrIj2asJEfB3Z7CehvkXrLmP2YyXoZB8AsIRAazcX2rlnBhMK5rF2tZZKoaSVl_T7LIS5Zo5YgVJxzAuef040cU6d4NEHuPGFU6aBI0qVDeQy3iR088Oi28cY7GFPeXBZBCIdk3AYYPTx2qtae66pYa4a1dtcRhlkvmkNSqWQqscBrCxDRZFZkiYzrWuRl9HBuNTsMQp1xQL3QzbR8Qz7IrzrDrc7f0T3dKv7tb_nS69NE3pd_MIp7nKICjNpzsDLIPCr4hTMoO3_ONL2GZkF_DYIuuMtuuYS4bbdoGg7PM70lU_BYtcXrcJi3el1nnVzYtz2fHK0ornOeKSXdGp9XYAXJ_d86wTWFslUGuxeXcd4apyw3Um6Pqwgt13eXrdXVWttWje31AM0tD1ROeJjouLDkIK3YS_RK6cNWpQiGa2HJ1HRoY0Unib8qoFxWbiMfnmjODyk2WM_zRrf9fFJn9Q5G3bWFGEWLVW_ynbfA7Qy4YLFuX_CYI8nLl1_49XEYfYAASk8sLDMDUujR_O_2czq6_LZbq1VB3DjoaQeq5pFr96ppgDOEpu5BzO_htE_z4O4QYPKCxWE87_wUA5jxGcgoAAA_22_7D = __webpack_require__(87737);
;// ./src/components/settings/HostKeyTrustDialog.css.ts


var actions = '_10aq6sa8';
var body = '_10aq6sa3';
var detailList = '_10aq6sa4';
var detailTerm = '_10aq6sa5';
var detailValue = '_10aq6sa6';
var dialog = '_10aq6sa1';
var HostKeyTrustDialog_css_fingerprint = '_10aq6sa7';
var heading = '_10aq6sa2';
var overlay = '_10aq6sa0';
var primaryButton = '_10aq6sa9';
var secondaryButton = '_10aq6saa';
;// ./src/components/settings/HostKeyTrustDialog.tsx
/* __next_internal_client_entry_do_not_use__ HostKeyTrustDialog auto */ 




/**
 * HostKeyTrustDialog — the TOFU (trust-on-first-use) confirmation modal
 * shown when TestRemoteConnection reports host_key_unknown=true (Task
 * 6.1.1c). Per ux.md Surface 3 / research/ux.md §1's VS Code precedent this
 * is a MODAL, not an inline prompt or a silent auto-trust: the remote is not
 * persisted until "Trust and connect" is explicitly clicked, and focus
 * starts on Cancel (not Trust) so a stray Enter keypress can't silently
 * trust an unverified host.
 *
 * Built on the same minimal focus-trap pattern as BackwardSyncConfirmDialog
 * (no existing shared primitive implements both Tab-cycling AND
 * focus-return-to-trigger — see that component's doc comment), extended
 * with backdrop-click-equals-Cancel per this dialog's own AC7 (a stronger
 * requirement than BackwardSyncConfirmDialog's, which doesn't need it).
 */ function HostKeyTrustDialog(param) {
    let { host, port, fingerprint, onTrust, onCancel } = param;
    const headingId = (0,react.useId)();
    const dialogRef = (0,react.useRef)(null);
    const cancelButtonRef = (0,react.useRef)(null);
    // Default focus lands on Cancel, not "Trust and connect" -- ux.md
    // Surface 3 interaction flow step 1.
    (0,useDialogFocusTrap/* useDialogFocusTrap */.L)({
        dialogRef,
        initialFocusRef: cancelButtonRef,
        onEscape: onCancel
    });
    const content = /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: overlay,
        "data-testid": "host-key-trust-overlay",
        onClick: (e)=>{
            // Backdrop click == Cancel, never Trust (ux.md Surface 3 AC7). Only
            // fires when the click target IS the overlay itself, not a bubbled
            // click from inside the dialog.
            if (e.target === e.currentTarget) onCancel();
        },
        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            ref: dialogRef,
            role: "dialog",
            "aria-modal": "true",
            "aria-labelledby": headingId,
            className: dialog,
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                    id: headingId,
                    className: heading,
                    children: "Verify host identity"
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                    className: body,
                    children: "Stapler Squad has not connected to this host before. Verify the fingerprint matches what the remote's administrator (you) expects."
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("dl", {
                    className: detailList,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("dt", {
                            className: detailTerm,
                            children: "Host"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("dd", {
                            className: detailValue,
                            "data-testid": "host-key-trust-host",
                            children: [
                                host,
                                ":",
                                port || 22
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("dt", {
                            className: detailTerm,
                            children: "Key type"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("dd", {
                            className: detailValue,
                            children: "ED25519"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("dt", {
                            className: detailTerm,
                            children: "Fingerprint"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("dd", {
                            className: [
                                detailValue,
                                HostKeyTrustDialog_css_fingerprint
                            ].join(" "),
                            "data-testid": "host-key-trust-fingerprint",
                            children: fingerprint
                        })
                    ]
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: actions,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            ref: cancelButtonRef,
                            type: "button",
                            className: secondaryButton,
                            onClick: onCancel,
                            "data-testid": "host-key-trust-cancel",
                            children: "Cancel"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            type: "button",
                            className: primaryButton,
                            onClick: onTrust,
                            "data-testid": "host-key-trust-confirm",
                            children: "Trust and connect"
                        })
                    ]
                })
            ]
        })
    });
    // createPortal escapes any ancestor CSS transform/filter/will-change that
    // would break position:fixed (ADR-009 / docs/reference/css-architecture.md).
    if (typeof document === "undefined") return null;
    return /*#__PURE__*/ (0,react_dom.createPortal)(content, document.body);
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsettings%2FAddRemoteForm.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA91WTY_aMBC98yt8qbQcvHJYCIu59J9Uxh4SC8eObAdCKv57hQk0EPJBtarUniLFM29m3jzP-H0n-HFVEfRzgpCQLlfsSNFWQbmeoPDFQlrgXhpNETeqyPT5JGE5RXtm3zD-Ea2gmtnqI5quJ6fJ-wUx-gLE2bGJOAuIW6M9drKCVvjNdH09P4BMUk9RTMj5HzfK2Ed70sT-GMBm006cWTjKmE2kpog0QOcBNGdCSJ203KrpYxASkDaM7xJrCi0eXaJLFhtjBViKorxEzigpHsx2DStsmZCFa9UTd9cTNXjs4TmTGqc1zfN5XjbrplvDCxeqr9N4GkkFIFN4JTVQpI2GOxQhHdsoEAHI5IxLf6SIvMch98K6M6Q2HjOlzAHEnTPNFeOQGiXABoDnxfKmChbPNcuUTDSWHjJHEQftwXYJtmqIAXvz3OAWL35RINHozg5178pebuS1nCFZZQPqzLoltWjfzMXlZvaoKKap2de9646q7_jslMxiWDLLf6kZ0GqGt0y7nFnQvnfk9XK-HMX5Z5Pzz57LNRsaaNGogRaR0VS_Omz7W3MrcvUVy6zqy3Bw-PNRVMEIppbNytiIkRfKdJ5Z31faDXJz2aUKSoqiq-AOUvg0rMea9C3LpDq2kIqBtvQs4r--PrvFFvbaHuxWmQMuKWKFN-d_h1R6wC5nHCjKbXPd8Rtr2KVW6l1N1svPh_9hIN1p5mFG8ddnlPjz111zodevu-8ZCMnQW8bKa4oxIXk5DVHunhJdIwJb2IN1cMY-_XaKa6caNSLk24PFssvi9AuE4-uPyQsAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsettings_2FAddRemoteForm_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA91WTY_aMBC98yt8qbQcvHJYCIu59J9Uxh4SC8eObAdCKv57hQk0EPJBtarUniLFM29m3jzP_H0n_HFVEfRzgpCQLlfsSNFWQbmeoPDFQlrgXhpNETeqyPT5JGE5RXtm3zD_Ea2gmtnqI5quJ6fJ_wUx_gLE2bGJOAuIW6M9drKCVvjNdH09P4BMUk9RTMj5HzfK2Ed70sT_GMBm006cWTjKmE2kpog0QOcBNGdCSJ203KrpYxASkDaM7xJrCi0eXaJLFhtjBViKorxEzigpHsx2DStsmZCFa9UTd9cTNXjs4TmTGqc1zfN5XjbrplvDCxeqr9N4GkkFIFN4JTVQpI2GOxQhHdsoEAHI5IxLf6SIvMch98K6M6Q2HjOlzAHEnTPNFeOQGiXABoDnxfKmChbPNcuUTDSWHjJHEQftwXYJtmqIAXvz3OAWL35RINHozg5178pebuS1nCFZZQPqzLoltWjfzMXlZvaoKKap2de9646q7_jslMxiWDLLf6kZ0GqGt0y7nFnQvnfk9XK_HMX5Z5Pzz57LNRsaaNGogRaR0VS_Omz7W3MrcvUVy6zqy3Bw_PNRVMEIppbNytiIkRfKdJ5Z31faDXJz2aUKSoqiq_AOUvg0rMea9C3LpDq2kIqBtvQs4r_PrvFFvbaHuxWmQMuKWKFN_d_h1R6wC5nHCjKbXPd8Rtr2KVW6l1N1svPh_9hIN1p5mFG8ddnlPjz111zodevu_8ZCMnQW8bKa4oxIXk5DVHunhJdIwJb2IN1cMY_XaKa6caNSLk24PFssvi9AuE4_uPyQsAAA_22_7D = __webpack_require__(63741);
;// ./src/components/settings/AddRemoteForm.css.ts


var actionRow = 'kdcy9z5';
var authorizedKeysBlock = 'kdcy9z9';
var authorizedKeysLine = 'kdcy9zb';
var authorizedKeysRow = 'kdcy9za';
var cancelBtn = 'kdcy9z7';
var caveat = 'kdcy9zd';
var copyBtn = 'kdcy9zc';
var errorMessage = 'kdcy9z8';
var fieldGroup = 'kdcy9z1';
var AddRemoteForm_css_form = 'kdcy9z0';
var hint = 'kdcy9z3';
var input = 'kdcy9z4';
var label = 'kdcy9z2';
var submitBtn = 'kdcy9z6';
;// ./src/components/settings/AddRemoteForm.tsx
/* __next_internal_client_entry_do_not_use__ AddRemoteForm auto */ 





/** Extracts a human-readable message from a failed RPC call. */ function AddRemoteForm_errorMessage(err) {
    if (err instanceof connect_error/* ConnectError */.T) return err.message;
    if (err instanceof Error) return err.message;
    return String(err);
}
/**
 * Add Remote form (Task 6.1.1b/e/f): name/host/user/port/base-path fields,
 * wired to TestRemoteConnection (draft mode) -> on unknown host key,
 * HostKeyTrustDialog -> TrustRemoteHostKey (draft mode) -> CreateRemote,
 * per ux.md Surface 2/3's interaction flow. The remote is never persisted
 * (CreateRemote is never called) until either an immediately-successful
 * test or an explicit "Trust and connect" click.
 */ function AddRemoteForm(param) {
    let { onSaved, onCancel } = param;
    const { createRemote, deleteRemote, generateRemoteIdentity, testRemoteConnectionDraft, trustRemoteHostKeyDraft } = (0,useRemotesService/* useRemotesService */.I)();
    const [name, setName] = (0,react.useState)("");
    const [host, setHost] = (0,react.useState)("");
    const [user, setUser] = (0,react.useState)("");
    const [port, setPort] = (0,react.useState)("");
    const [basePath, setBasePath] = (0,react.useState)("");
    const [identity, setIdentity] = (0,react.useState)(null);
    const [testing, setTesting] = (0,react.useState)(false);
    const [error, setError] = (0,react.useState)(null);
    const [copied, setCopied] = (0,react.useState)(false);
    // Set only while a host_key_unknown response is pending user action --
    // the dialog is rendered whenever this is non-empty.
    const [pendingFingerprint, setPendingFingerprint] = (0,react.useState)(null);
    const [trustBusy, setTrustBusy] = (0,react.useState)(false);
    // Guards against handleTrust acting on a stale result after the user has
    // already clicked Cancel while trustRemoteHostKeyDraft was still in
    // flight -- without this, the dialog visibly closes on Cancel but the
    // remote is still persisted (and onSaved still fires) once the pending
    // RPC resolves behind the user's back. A ref rather than a `cancelled`
    // local (useConfiguredRemotes.ts's equivalent idiom) because the
    // cancellation and the in-flight await live in two different callbacks
    // (handleTrustCancel vs. handleTrust), not one effect and its own cleanup.
    const trustCancelledRef = (0,react.useRef)(false);
    const canSubmit = Boolean(name.trim()) && Boolean(host.trim()) && Boolean(user.trim()) && Boolean(basePath.trim()) && !testing;
    // Once an identity has been generated, EVERY field locks (ux.md Surface 2:
    // "fields as above, now read-only until Cancel/retry") -- not just while
    // `testing` is true. Identity is generated once, keyed by `name`
    // (GenerateOrDescribeIdentity), and the displayed authorized_keys line is
    // that exact key. If fields (especially Name) stayed editable after
    // generation, a rename followed by Create would call CreateRemote with a
    // NEW name that has no stored identity -- its GenerateOrDescribeIdentity
    // fallback would then silently mint a FRESH, different keypair than the
    // one shown/pasted, and the saved remote's stored identity would never
    // match what's actually in the remote's authorized_keys. Fields stay
    // locked until the user explicitly cancels the whole form (which deletes
    // the orphaned identity) -- there is no "unlock to edit, then retry" path.
    const fieldsLocked = testing || Boolean(identity);
    const portNumber = port.trim() === "" ? 0 : Number(port);
    const currentDraft = (0,react.useCallback)(()=>({
            name: name.trim(),
            host: host.trim(),
            user: user.trim(),
            port: portNumber
        }), [
        name,
        host,
        user,
        portNumber
    ]);
    const persistRemote = (0,react.useCallback)(async ()=>{
        const saved = await createRemote({
            name: name.trim(),
            host: host.trim(),
            user: user.trim(),
            port: portNumber,
            basePath: basePath.trim()
        });
        onSaved(saved);
    }, [
        createRemote,
        name,
        host,
        user,
        portNumber,
        basePath,
        onSaved
    ]);
    const handleTestConnection = (0,react.useCallback)(async (e)=>{
        e.preventDefault();
        if (!canSubmit) return;
        setError(null);
        setTesting(true);
        try {
            let currentIdentity = identity;
            if (!currentIdentity) {
                currentIdentity = await generateRemoteIdentity(name.trim());
                setIdentity(currentIdentity);
            }
            const result = await testRemoteConnectionDraft(currentDraft());
            if (result.hostKeyUnknown) {
                setPendingFingerprint(result.fingerprint);
            } else if (result.success) {
                await persistRemote();
            } else {
                setError(result.errorMessage || "Couldn't reach ".concat(name.trim(), ". Check that the host is up, or pick a different remote."));
            }
        } catch (err) {
            setError(AddRemoteForm_errorMessage(err));
        } finally{
            setTesting(false);
        }
    }, [
        canSubmit,
        identity,
        name,
        generateRemoteIdentity,
        testRemoteConnectionDraft,
        currentDraft,
        persistRemote
    ]);
    const handleTrust = (0,react.useCallback)(async ()=>{
        if (!pendingFingerprint) return;
        trustCancelledRef.current = false;
        setTrustBusy(true);
        setError(null);
        try {
            const trustResult = await trustRemoteHostKeyDraft(currentDraft(), pendingFingerprint);
            // Bail out if the user clicked Cancel while this RPC was still in
            // flight -- otherwise a resolve-after-cancel would silently persist
            // the remote (and fire onSaved) even though the dialog already closed.
            // See trustCancelledRef's declaration above.
            if (trustCancelledRef.current) return;
            if (trustResult.success) {
                setPendingFingerprint(null);
                await persistRemote();
            } else {
                setPendingFingerprint(null);
                setError(trustResult.errorMessage || "Trusting the host key failed.");
            }
        } catch (err) {
            if (trustCancelledRef.current) return;
            setPendingFingerprint(null);
            setError(AddRemoteForm_errorMessage(err));
        } finally{
            setTrustBusy(false);
        }
    }, [
        pendingFingerprint,
        trustRemoteHostKeyDraft,
        currentDraft,
        persistRemote
    ]);
    const handleTrustCancel = (0,react.useCallback)(()=>{
        // Signal any in-flight handleTrust call to bail out once its await
        // resolves, instead of silently persisting the remote after the user
        // already closed the dialog -- see handleTrust's trustCancelledRef check.
        trustCancelledRef.current = true;
        // Remote was never saved (CreateRemote is only called after a
        // successful Trust) -- closing the dialog just returns to the still-
        // editable Add Remote form. Per ux.md Surface 3 step 4, this is not a
        // dead end: the user can fix things on the remote host and retry Test
        // connection, which reuses the SAME generated identity (see
        // GenerateRemoteIdentity's idempotency).
        setPendingFingerprint(null);
    }, []);
    const handleFormCancel = (0,react.useCallback)(()=>{
        // If an identity was generated but the remote was never saved, clean up
        // the orphaned keychain entry (ux.md Surface 2, interaction flow step 3)
        // -- best-effort, a failure here shouldn't block closing the form.
        if (identity && name.trim()) {
            void deleteRemote(name.trim()).catch(()=>{});
        }
        onCancel();
    }, [
        identity,
        name,
        deleteRemote,
        onCancel
    ]);
    const handleCopyAuthorizedKeys = (0,react.useCallback)(()=>{
        if (!identity) return;
        void navigator.clipboard.writeText(identity.authorizedKeysLine).then(()=>{
            setCopied(true);
            window.setTimeout(()=>setCopied(false), 2000);
        });
    }, [
        identity
    ]);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("form", {
                className: AddRemoteForm_css_form,
                onSubmit: handleTestConnection,
                "data-testid": "add-remote-form",
                children: [
                    error && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: errorMessage,
                        role: "alert",
                        "data-testid": "add-remote-error",
                        children: error
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: fieldGroup,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                className: label,
                                htmlFor: "add-remote-name",
                                children: [
                                    "Name",
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        "aria-hidden": "true",
                                        children: "*"
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                id: "add-remote-name",
                                type: "text",
                                className: input,
                                value: name,
                                onChange: (e)=>setName(e.target.value),
                                disabled: fieldsLocked,
                                required: true,
                                "aria-required": "true",
                                placeholder: "prod-box",
                                "data-testid": "add-remote-name"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: fieldGroup,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                className: label,
                                htmlFor: "add-remote-host",
                                children: [
                                    "Host",
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        "aria-hidden": "true",
                                        children: "*"
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                id: "add-remote-host",
                                type: "text",
                                className: input,
                                value: host,
                                onChange: (e)=>setHost(e.target.value),
                                disabled: fieldsLocked,
                                required: true,
                                "aria-required": "true",
                                placeholder: "prod.example.com",
                                "data-testid": "add-remote-host"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: fieldGroup,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                className: label,
                                htmlFor: "add-remote-user",
                                children: [
                                    "User",
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        "aria-hidden": "true",
                                        children: "*"
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                id: "add-remote-user",
                                type: "text",
                                className: input,
                                value: user,
                                onChange: (e)=>setUser(e.target.value),
                                disabled: fieldsLocked,
                                required: true,
                                "aria-required": "true",
                                placeholder: "tyler",
                                "data-testid": "add-remote-user"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: fieldGroup,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                className: label,
                                htmlFor: "add-remote-port",
                                children: "Port"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                id: "add-remote-port",
                                type: "number",
                                inputMode: "numeric",
                                className: input,
                                value: port,
                                onChange: (e)=>setPort(e.target.value),
                                disabled: fieldsLocked,
                                placeholder: "22",
                                "data-testid": "add-remote-port"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: fieldGroup,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                className: label,
                                htmlFor: "add-remote-base-path",
                                children: [
                                    "Base path",
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        "aria-hidden": "true",
                                        children: "*"
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                id: "add-remote-base-path",
                                type: "text",
                                className: input,
                                value: basePath,
                                onChange: (e)=>setBasePath(e.target.value),
                                disabled: fieldsLocked,
                                required: true,
                                "aria-required": "true",
                                placeholder: "/srv/workspaces",
                                "data-testid": "add-remote-base-path"
                            })
                        ]
                    }),
                    !identity && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: hint,
                        children: "A new SSH keypair will be generated for this remote and stored in your OS keychain. You'll need to add the public key below to the remote's authorized_keys."
                    }),
                    identity && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: authorizedKeysBlock,
                        "data-testid": "add-remote-authorized-keys",
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                className: label,
                                children: [
                                    "Add this to ~/.ssh/authorized_keys on ",
                                    host.trim() || "the remote",
                                    ":"
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: authorizedKeysRow,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("code", {
                                        className: authorizedKeysLine,
                                        children: identity.authorizedKeysLine
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        type: "button",
                                        className: copyBtn,
                                        onClick: handleCopyAuthorizedKeys,
                                        "data-testid": "add-remote-copy-authorized-keys",
                                        children: copied ? "Copied!" : "Copy"
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                className: caveat,
                                children: "Stapler Squad cannot verify this line was applied — test the connection once it's in place."
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: actionRow,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: cancelBtn,
                                onClick: handleFormCancel,
                                "data-testid": "add-remote-cancel",
                                children: "Cancel"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "submit",
                                className: submitBtn,
                                disabled: !canSubmit,
                                "data-testid": "add-remote-submit",
                                children: testing ? "Testing…" : "Test connection"
                            })
                        ]
                    })
                ]
            }),
            pendingFingerprint && /*#__PURE__*/ (0,jsx_runtime.jsx)(HostKeyTrustDialog, {
                host: host.trim(),
                port: portNumber,
                fingerprint: pendingFingerprint,
                onTrust: ()=>{
                    if (!trustBusy) void handleTrust();
                },
                onCancel: handleTrustCancel
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fapp%2Fsettings%2Fremotes%2FRemotesPage.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA8VXy3LbIBTd5yvYdCZeKIPkV0I3_ZMMFtcSMQIVcGS5k3_vgO1WSNarzbQba8zj3HvPPRykp9f4--GwqpYY_XhAqKCnqOLM5gRtE1yevj4glAPPcktQjPEX91-9g94LVUU1QfRolRtj3JSC1gTtBfhN7hkxriG1XEmCUiWOhXQzGS0Jeqf6MYpe4xc4J_q8TBZupqSMcZndnf14eLplGvtMOxGp4JmMuIXCEJSCtKDd8NvRWL6vo1RJC9ISZEqaQrQDWwH0JoTDkIkPuVfSRoafobOc-fz9fHVla4OxG0uVULq9HvvlBdUZlwThINRyJNRu0QubDMCuPGwPw8l5cY8AhHZKM9CRpowfTSeVze-qh1I9auNyLRW_9eQCS1BcnpBRgrPWzuISnKaHTKujZG3kop-DdbcT60snCi6jm5RXKyftJj0kd7L2JPXHlaEq1veFOEv6yTnE3MzFDGTvlxlLtR0S9vBJGyJgu2i3j1GTQ7t_MEE827Du7axDPb8yPLuye8KcUtgmLOz5M0RSL37tybSqCIpvir6adXjYXyaQuaMGBJcwpMtryEq7efcbRKGTnGqWKTbQd3_ngw2kdASJ9iPFIRK7z-sIfSbXXB58j3oE3QgBf152i8D9oOPXbce_ZvyfHB86x9NqKk1JNUg7do92jD04Gh2v30_y-ucWm4RxQ3cCLhpQJU25rQnCT-tmyVLZiAqhKmDB9uwSbYSFGA9IsaXqbFIVcUvBeZBH0pfHcsQtrwum3sBbHPoT91kMvMNMMhVjawEEcUsFTwP8t378eOyK84xNbtWU4zL3pWu4-EaZh0-4Wpbx4LUZ_-Nr81sBjFP02PgO2WBcnha-1NYnwFCpLac1VoNNczfz0QTafhYQuwJ1Pjc8Mkh22fHxE7D_bRxwDQAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fapp_2Fsettings_2Fremotes_2FRemotesPage_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA8VXy3LbIBTd5yvYdCZeKIPkV0I3_ZMMFtcSMQIVcGS5k3_vgO1WSNarzbQba8zj3HvPPRykp9f4_GwqpYY_XhAqKCnqOLM5gRtE1yevj4glAPPcktQjPEX91_9g94LVUU1QfRolRtj3JSC1gTtBfhN7hkxriG1XEmCUiWOhXQzGS0Jeqf6MYpe4xc4J_q8TBZupqSMcZndnf14eLplGvtMOxGp4JmMuIXCEJSCtKDd8NvRWL6vo1RJC9ISZEqaQrQDWwH0JoTDkIkPuVfSRoafobOc_fz9fHVla4OxG0uVULq9HvvlBdUZlwThINRyJNRu0QubDMCuPGwPw8l5cY8AhHZKM9CRpowfTSeVze_qh1I9auNyLRW_9eQCS1BcnpBRgrPWzuISnKaHTKujZG3kop_DdbcT60snCi6jm5RXKyftJj0kd7L2JPXHlaEq1veFOEv6yTnE3MzFDGTvlxlLtR0S9vBJGyJgu2i3j1GTQ7t_MEE827Du7axDPb8yPLuye8KcUtgmLOz5M0RSL37tybSqCIpvir6adXjYXyaQuaMGBJcwpMtryEq7efcbRKGTnGqWKTbQd3_ngw2kdASJ9iPFIRK7z_sIfSbXXB58j3oE3QgBf152i8D9oOPXbce_ZvyfHB86x9NqKk1JNUg7do92jD04Gh2v30_y_ucWm4RxQ3cCLhpQJU25rQnCT_tmyVLZiAqhKmDB9uwSbYSFGA9IsaXqbFIVcUvBeZBH0pfHcsQtrwum3sBbHPoT91kMvMNMMhVjawEEcUsFTwP8t378eOyK84xNbtWU4zL3pWu4_EaZh0_4Wpbx4LUZ_Nr81sBjFP02PgO2WBcnha_1NYnwFCpLac1VoNNczfz0QTafhYQuwJ1Pjc8Mkh22fHxE7D_bRxwDQAA_22_7D = __webpack_require__(18464);
;// ./src/app/settings/remotes/RemotesPage.css.ts


var RemotesPage_css_actionRow = '_1qkk4w3d';
var confirmDeleteBtn = '_1qkk4w3h _1qkk4w3f';
var confirmText = '_1qkk4w3e';
var container = '_1qkk4w30';
var deleteBtn = '_1qkk4w3g _1qkk4w3f';
var description = '_1qkk4w33';
var empty = '_1qkk4w3i';
var emptyState = '_1qkk4w36';
var RemotesPage_css_errorMessage = '_1qkk4w3j';
var formOverlay = '_1qkk4w3k';
var headerRow = '_1qkk4w31';
var list = '_1qkk4w35';
var listItem = '_1qkk4w37';
var listItemInfo = '_1qkk4w38';
var listItemMeta = '_1qkk4w3b';
var listItemName = '_1qkk4w3a';
var listItemNameRow = '_1qkk4w39';
var listItemStatus = '_1qkk4w3c';
var newBtn = '_1qkk4w34';
var smallBtn = '_1qkk4w3f';
var title = '_1qkk4w32';
;// ./src/app/settings/remotes/page.tsx
// analytics-exempt
/* __next_internal_client_entry_do_not_use__ default auto */ 
// +feature: settings-remotes
//
// PageViewTracker (rendered below) calls usePageView() internally, so this
// page is exempt from analytics/require-page-analytics's literal-call check
// -- same pattern as pipeline-modes/page.tsx.






function page_errorMessage(err) {
    if (err instanceof connect_error/* ConnectError */.T) return err.message;
    if (err instanceof Error) return err.message;
    return String(err);
}
/**
 * Settings -> Remotes: lists configured SSH remotes with row-level Test/
 * Delete actions, and an "Add remote" form (ssh-remote-workspaces Phase 6,
 * Epic 6.1, Task 6.1.1a). Covers requirements.md AC1's UI half.
 */ function RemotesPage() {
    const { listRemotes, deleteRemote, testRemoteConnectionSaved } = (0,useRemotesService/* useRemotesService */.I)();
    const [remotes, setRemotes] = (0,react.useState)([]);
    const [loading, setLoading] = (0,react.useState)(true);
    const [loadError, setLoadError] = (0,react.useState)(null);
    const [formOpen, setFormOpen] = (0,react.useState)(false);
    const [testingName, setTestingName] = (0,react.useState)(null);
    const [testStatus, setTestStatus] = (0,react.useState)({});
    const [confirmDeleteName, setConfirmDeleteName] = (0,react.useState)(null);
    const [deleting, setDeleting] = (0,react.useState)(false);
    const refresh = (0,react.useCallback)(async ()=>{
        setLoading(true);
        setLoadError(null);
        try {
            const list = await listRemotes();
            setRemotes(list);
        } catch (err) {
            setLoadError(page_errorMessage(err));
        } finally{
            setLoading(false);
        }
    }, [
        listRemotes
    ]);
    (0,react.useEffect)(()=>{
        refresh();
    }, [
        refresh
    ]);
    const handleSaved = (0,react.useCallback)((remote)=>{
        setRemotes((prev)=>{
            const idx = prev.findIndex((r)=>r.name === remote.name);
            if (idx === -1) return [
                ...prev,
                remote
            ];
            const next = [
                ...prev
            ];
            next[idx] = remote;
            return next;
        });
        setFormOpen(false);
    }, []);
    const handleTest = (0,react.useCallback)(async (name)=>{
        setTestingName(name);
        setTestStatus((prev)=>({
                ...prev,
                [name]: ""
            }));
        try {
            const result = await testRemoteConnectionSaved(name);
            if (result.success) {
                setTestStatus((prev)=>({
                        ...prev,
                        [name]: "Connected"
                    }));
            } else if (result.hostKeyUnknown) {
                setTestStatus((prev)=>({
                        ...prev,
                        [name]: "Host key changed or not yet trusted — re-add this remote to re-trust it."
                    }));
            } else {
                setTestStatus((prev)=>({
                        ...prev,
                        [name]: result.errorMessage || "Connection failed."
                    }));
            }
        } catch (err) {
            setTestStatus((prev)=>({
                    ...prev,
                    [name]: page_errorMessage(err)
                }));
        } finally{
            setTestingName(null);
        }
    }, [
        testRemoteConnectionSaved
    ]);
    const handleDeleteConfirm = (0,react.useCallback)(async (name)=>{
        setDeleting(true);
        try {
            await deleteRemote(name);
            setRemotes((prev)=>prev.filter((r)=>r.name !== name));
            setConfirmDeleteName(null);
        } catch (err) {
            setLoadError(page_errorMessage(err));
        } finally{
            setDeleting(false);
        }
    }, [
        deleteRemote
    ]);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)(PageViewTracker.PageViewTracker, {}),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("main", {
                id: "main-content",
                className: container,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: headerRow,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h1", {
                                        className: title,
                                        children: "Remotes"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                        className: description,
                                        children: "Register a remote host to run sessions on a dedicated Linux box instead of this machine."
                                    })
                                ]
                            }),
                            !formOpen && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                className: newBtn,
                                onClick: ()=>setFormOpen(true),
                                "data-testid": "remotes-add-button",
                                children: "+ Add remote"
                            })
                        ]
                    }),
                    loadError && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: RemotesPage_css_errorMessage,
                        role: "alert",
                        children: loadError
                    }),
                    formOpen && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: formOverlay,
                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)(AddRemoteForm, {
                            onSaved: handleSaved,
                            onCancel: ()=>setFormOpen(false)
                        })
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: list,
                        children: loading ? /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                            className: empty,
                            children: "Loading…"
                        }) : remotes.length === 0 ? !formOpen && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: emptyState,
                            "data-testid": "remotes-empty-state",
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                    className: empty,
                                    children: "No remotes configured yet."
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                    className: description,
                                    children: "Register a remote host to run sessions on a dedicated Linux box instead of this machine."
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    className: newBtn,
                                    onClick: ()=>setFormOpen(true),
                                    children: "+ Add remote"
                                })
                            ]
                        }) : remotes.map((r)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: listItem,
                                "data-testid": "remote-row-".concat(r.name),
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        className: listItemInfo,
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                className: listItemNameRow,
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: listItemName,
                                                        children: r.name
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                        className: listItemMeta,
                                                        children: [
                                                            r.user,
                                                            "@",
                                                            r.host,
                                                            r.port ? ":".concat(r.port) : ""
                                                        ]
                                                    })
                                                ]
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: listItemMeta,
                                                children: r.basePath
                                            }),
                                            testStatus[r.name] && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: listItemStatus,
                                                role: "status",
                                                "aria-live": "polite",
                                                "data-testid": "remote-status-".concat(r.name),
                                                children: testStatus[r.name]
                                            })
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                        className: RemotesPage_css_actionRow,
                                        children: confirmDeleteName === r.name ? /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                    className: confirmText,
                                                    children: [
                                                        "Remove ",
                                                        r.name,
                                                        "?"
                                                    ]
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                    className: confirmDeleteBtn,
                                                    onClick: ()=>handleDeleteConfirm(r.name),
                                                    disabled: deleting,
                                                    "data-testid": "remote-confirm-delete-".concat(r.name),
                                                    children: deleting ? "Removing…" : "Remove"
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                    className: smallBtn,
                                                    onClick: ()=>setConfirmDeleteName(null),
                                                    disabled: deleting,
                                                    "data-testid": "remote-cancel-delete-".concat(r.name),
                                                    children: "Cancel"
                                                })
                                            ]
                                        }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                    className: smallBtn,
                                                    onClick: ()=>handleTest(r.name),
                                                    disabled: testingName === r.name,
                                                    "data-testid": "remote-test-".concat(r.name),
                                                    children: testingName === r.name ? "Testing…" : "Test"
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                    className: deleteBtn,
                                                    onClick: ()=>setConfirmDeleteName(r.name),
                                                    "aria-label": "Delete ".concat(r.name),
                                                    "data-testid": "remote-delete-".concat(r.name),
                                                    children: "Delete"
                                                })
                                            ]
                                        })
                                    })
                                ]
                            }, r.name))
                    })
                ]
            })
        ]
    });
}


/***/ }),

/***/ 21694:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   O: () => (/* binding */ getKeepaliveConnectTransport),
/* harmony export */   VZ: () => (/* binding */ getWatchTransport),
/* harmony export */   qW: () => (/* binding */ getConnectTransport)
/* harmony export */ });
/* unused harmony export _resetTransportForTesting */
/* harmony import */ var _connectrpc_connect_web__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(46564);
/* harmony import */ var _lib_config__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(51202);
/* harmony import */ var _lib_transport_watch_ws_transport__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(90062);



let _transport = null;
let _watchTransport = null;
let _keepaliveTransport = null;
/**
 * Every unary RPC through this transport sends a Connect-Timeout-Ms header
 * with this value unless a call site passes its own timeoutMs — connect-go
 * reads that header and derives the handler's ctx deadline automatically, so
 * this is what makes "the backend can tell a caller disconnected" true for
 * ordinary RPCs (streamViaHub's WebSocket path uses connection-scoped
 * cancellation instead, since a live terminal stream has no fixed deadline).
 */ const DEFAULT_RPC_TIMEOUT_MS = 30000;
/**
 * Returns the shared ConnectRPC HTTP transport singleton.
 *
 * Every non-streaming hook should call this instead of constructing its own
 * transport. This ensures a single transport instance for all unary/streaming
 * non-watch RPCs, carrying the auth interceptor (redirects to /login on a
 * 401) in one place instead of every call site wiring it up separately.
 *
 * The streaming watch transport (createWatchTransport) is separate and should
 * NOT use this singleton — it has its own reconnect logic. Use
 * getWatchTransport() for that.
 */ function getConnectTransport() {
    if (!_transport) {
        _transport = (0,_connectrpc_connect_web__WEBPACK_IMPORTED_MODULE_2__/* .createConnectTransport */ .w)({
            baseUrl: (0,_lib_config__WEBPACK_IMPORTED_MODULE_0__/* .getApiBaseUrl */ .h)(),
            defaultTimeoutMs: DEFAULT_RPC_TIMEOUT_MS,
            interceptors: [
                (0,_lib_config__WEBPACK_IMPORTED_MODULE_0__/* .createAuthInterceptor */ ._)()
            ]
        });
    }
    return _transport;
}
/**
 * A unary transport whose requests use `fetch(..., { keepalive: true })`, so a
 * request started as the page hides or unloads still reaches the server. Bodies
 * must stay under the browser's 64KB keepalive limit; only use it for small
 * "complete this action now" RPCs (the tray's deferred clear).
 */ function getKeepaliveConnectTransport() {
    if (!_keepaliveTransport) {
        _keepaliveTransport = (0,_connectrpc_connect_web__WEBPACK_IMPORTED_MODULE_2__/* .createConnectTransport */ .w)({
            baseUrl: (0,_lib_config__WEBPACK_IMPORTED_MODULE_0__/* .getApiBaseUrl */ .h)(),
            defaultTimeoutMs: DEFAULT_RPC_TIMEOUT_MS,
            interceptors: [
                (0,_lib_config__WEBPACK_IMPORTED_MODULE_0__/* .createAuthInterceptor */ ._)()
            ],
            // Cast: Bun's global fetch type also requires `preconnect`, which a wrapper cannot provide (tsconfig.bun.json).
            fetch: (input, init)=>fetch(input, {
                    ...init,
                    keepalive: true
                })
        });
    }
    return _keepaliveTransport;
}
/**
 * Returns the shared transport singleton for server-streaming Watch* RPCs.
 *
 * Selects WebSocket-bridge vs. native ConnectRPC streaming per
 * createSessionWatchTransport's rules (see watch-ws-transport.ts) — every
 * Watch* hook should call this instead of constructing its own transport, so
 * that choice and the auth interceptor live in one place.
 */ function getWatchTransport() {
    if (!_watchTransport) {
        _watchTransport = (0,_lib_transport_watch_ws_transport__WEBPACK_IMPORTED_MODULE_1__/* .createSessionWatchTransport */ .ZX)({
            baseUrl: (0,_lib_config__WEBPACK_IMPORTED_MODULE_0__/* .getApiBaseUrl */ .h)(),
            interceptors: [
                (0,_lib_config__WEBPACK_IMPORTED_MODULE_0__/* .createAuthInterceptor */ ._)()
            ]
        });
    }
    return _watchTransport;
}
/** Reset the transport singletons (for testing only). */ function _resetTransportForTesting() {
    _transport = null;
    _watchTransport = null;
    _keepaliveTransport = null;
}


/***/ }),

/***/ 24044:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Bl: () => (/* binding */ AnalyticsContextProvider),
/* harmony export */   Ig: () => (/* binding */ AnalyticsContext),
/* harmony export */   st: () => (/* binding */ useAnalytics)
/* harmony export */ });
/* harmony import */ var react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(61574);
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(55494);
/* __next_internal_client_entry_do_not_use__ AnalyticsContext,useAnalytics,AnalyticsContextProvider auto */ 

const AnalyticsContext = /*#__PURE__*/ (0,react__WEBPACK_IMPORTED_MODULE_1__.createContext)(null);
function useAnalytics() {
    const context = (0,react__WEBPACK_IMPORTED_MODULE_1__.useContext)(AnalyticsContext);
    if (!context) {
        throw new Error("useAnalytics must be used within an AnalyticsContextProvider");
    }
    return context;
}
function AnalyticsContextProvider(param) {
    let { provider, children } = param;
    // Stable ref to the provider so identity doesn't change across parent re-renders
    const providerRef = (0,react__WEBPACK_IMPORTED_MODULE_1__.useRef)(provider);
    providerRef.current = provider;
    (0,react__WEBPACK_IMPORTED_MODULE_1__.useEffect)(()=>{
        var _p_initialize;
        const p = provider;
        providerRef.current = p;
        void ((_p_initialize = p.initialize) === null || _p_initialize === void 0 ? void 0 : _p_initialize.call(p));
        return ()=>{
            var _p_onClose;
            void ((_p_onClose = p.onClose) === null || _p_onClose === void 0 ? void 0 : _p_onClose.call(p));
        };
    }, [
        provider
    ]);
    const contextValue = (0,react__WEBPACK_IMPORTED_MODULE_1__.useMemo)(()=>({
            provider: providerRef.current,
            track: (event)=>providerRef.current.track(event)
        }), // track delegates through the ref so consumers don't re-render on provider swap
    // eslint-disable-next-line react-hooks/exhaustive-deps
    []);
    return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)(AnalyticsContext.Provider, {
        value: contextValue,
        children: children
    });
}


/***/ }),

/***/ 41078:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   KA: () => (/* reexport safe */ _lib_nav_impl__WEBPACK_IMPORTED_MODULE_0__.useRouter),
/* harmony export */   N_: () => (/* reexport safe */ _lib_nav_impl__WEBPACK_IMPORTED_MODULE_0__.Link),
/* harmony export */   a8: () => (/* reexport safe */ _lib_nav_impl__WEBPACK_IMPORTED_MODULE_0__.usePathname),
/* harmony export */   nQ: () => (/* reexport safe */ _lib_nav_impl__WEBPACK_IMPORTED_MODULE_0__.replaceHash),
/* harmony export */   ok: () => (/* reexport safe */ _lib_nav_impl__WEBPACK_IMPORTED_MODULE_0__.useSearchParams),
/* harmony export */   tk: () => (/* reexport safe */ _lib_nav_impl__WEBPACK_IMPORTED_MODULE_0__.pushHistoryMarker),
/* harmony export */   wV: () => (/* reexport safe */ _lib_nav_impl__WEBPACK_IMPORTED_MODULE_0__.useReplaceSearch),
/* harmony export */   x7: () => (/* reexport safe */ _lib_nav_impl__WEBPACK_IMPORTED_MODULE_0__.pushTrayEntry)
/* harmony export */ });
/* harmony import */ var _lib_nav_impl__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(99863);
// `@/lib/nav/impl` is aliased per build (next.config.ts, tsconfig paths, jest moduleNameMapper); everything outside this directory imports from here.




/***/ }),

/***/ 51202:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   _: () => (/* binding */ createAuthInterceptor),
/* harmony export */   h: () => (/* binding */ getApiBaseUrl)
/* harmony export */ });
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(45640);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(16294);
/* provided dependency */ var process = __webpack_require__(36843);

/**
 * Get the API base URL for the application
 *
 * In production/test: Uses the current window location with /api prefix
 * In development: Falls back to localhost:8543/api
 *
 * This ensures tests running on port 8544 connect to the test server,
 * while development and production use appropriate URLs.
 * All ConnectRPC services are mounted under /api/ prefix.
 */ function getApiBaseUrl() {
    // Explicit override always wins, even in the browser (e.g. `next dev`
    // running on its own port against a separately-ported backend).
    if (process.env.NEXT_PUBLIC_API_URL) {
        return process.env.NEXT_PUBLIC_API_URL;
    }
    // In browser environment
    if (true) {
        // Use the current origin (hostname + port) with /api prefix
        return window.location.origin + '/api';
    }
    // Fallback for server-side rendering or development
    return 'http://localhost:8543/api';
}
/**
 * ConnectRPC interceptor that redirects to /login whenever a call fails
 * with Code.Unauthenticated (HTTP 401).  Attach this to every transport so
 * that expired or wiped sessions surface as a login redirect rather than a
 * silent background failure.
 */ function createAuthInterceptor() {
    return (next)=>async (req)=>{
            try {
                return await next(req);
            } catch (err) {
                if ( true && !window.location.pathname.startsWith('/login') && err instanceof _connectrpc_connect__WEBPACK_IMPORTED_MODULE_0__/* .ConnectError */ .T && err.code === _connectrpc_connect__WEBPACK_IMPORTED_MODULE_1__/* .Code */ .C.Unauthenticated) {
                    window.location.href = '/login';
                }
                throw err;
            }
        };
}


/***/ }),

/***/ 52135:
/***/ ((__unused_webpack_module, __unused_webpack_exports, __webpack_require__) => {

Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 18678));


/***/ }),

/***/ 63741:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 65916:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 82279:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   L: () => (/* binding */ useDialogFocusTrap)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);

const FOCUSABLE_SELECTOR = 'a[href], button:not([disabled]), textarea:not([disabled]), input:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])';
/**
 * Shared modal focus-trap behavior: focuses `initialFocusRef` on mount,
 * restores focus to whatever was focused before the dialog opened once it
 * unmounts, and cycles Tab/Shift+Tab within `dialogRef` while routing
 * Escape to `onEscape`.
 *
 * Extracted from BackwardSyncConfirmDialog and HostKeyTrustDialog, which
 * both implemented this identically apart from which button receives
 * initial focus (Confirm vs Cancel).
 */ function useDialogFocusTrap(param) {
    let { dialogRef, initialFocusRef, onEscape } = param;
    // Capture whatever had focus when the dialog mounted (the element that
    // triggered it) so focus can be restored on close.
    const previouslyFocusedRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(typeof document !== "undefined" ? document.activeElement : null);
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        var _initialFocusRef_current;
        (_initialFocusRef_current = initialFocusRef.current) === null || _initialFocusRef_current === void 0 ? void 0 : _initialFocusRef_current.focus();
        return ()=>{
            var _previouslyFocusedRef_current;
            (_previouslyFocusedRef_current = previouslyFocusedRef.current) === null || _previouslyFocusedRef_current === void 0 ? void 0 : _previouslyFocusedRef_current.focus();
        };
    // eslint-disable-next-line react-hooks/exhaustive-deps -- run once on mount/unmount only
    }, []);
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        const handleKeyDown = (e)=>{
            if (e.key === "Escape") {
                e.preventDefault();
                onEscape();
                return;
            }
            if (e.key === "Tab" && dialogRef.current) {
                const focusable = Array.from(dialogRef.current.querySelectorAll(FOCUSABLE_SELECTOR));
                if (focusable.length === 0) return;
                const first = focusable[0];
                const last = focusable[focusable.length - 1];
                if (e.shiftKey && document.activeElement === first) {
                    e.preventDefault();
                    last.focus();
                } else if (!e.shiftKey && document.activeElement === last) {
                    e.preventDefault();
                    first.focus();
                }
            }
        };
        document.addEventListener("keydown", handleKeyDown);
        return ()=>document.removeEventListener("keydown", handleKeyDown);
    }, [
        dialogRef,
        onEscape
    ]);
}


/***/ }),

/***/ 87737:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 88149:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   CO: () => (/* binding */ RemoteConnectionState),
/* harmony export */   Cj: () => (/* binding */ ListRemotesRequestSchema),
/* harmony export */   Ek: () => (/* binding */ TrustRemoteHostKeyRequestSchema),
/* harmony export */   Jt: () => (/* binding */ TestRemoteConnectionRequestSchema),
/* harmony export */   TW: () => (/* binding */ CreateRemoteRequestSchema),
/* harmony export */   ag: () => (/* binding */ DraftRemoteTargetSchema),
/* harmony export */   cj: () => (/* binding */ GenerateRemoteIdentityRequestSchema),
/* harmony export */   eB: () => (/* binding */ DeleteRemoteRequestSchema),
/* harmony export */   ef: () => (/* binding */ RemoteService),
/* harmony export */   hp: () => (/* binding */ file_session_v1_remote)
/* harmony export */ });
/* unused harmony exports RemoteConfigProtoSchema, TestRemoteConnectionResponseSchema, TrustRemoteHostKeyResponseSchema, GenerateRemoteIdentityResponseSchema, ListRemotesResponseSchema, CreateRemoteResponseSchema, DeleteRemoteResponseSchema, RemoteConnectionStateSchema */
/* harmony import */ var _bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(1844);
/* harmony import */ var _bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(15033);
/* harmony import */ var _bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(95765);
// @generated by protoc-gen-es v2.11.0 with parameter "target=ts,ts_nocheck=false,keep_empty_files=true"
// @generated from file session/v1/remote.proto (package session.v1, syntax proto3)
/* eslint-disable */ 
/**
 * Describes the file session/v1/remote.proto.
 */ const file_session_v1_remote = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_0__/* .fileDesc */ .w)("ChdzZXNzaW9uL3YxL3JlbW90ZS5wcm90bxIKc2Vzc2lvbi52MSJ0ChFSZW1vdGVDb25maWdQcm90bxIMCgRuYW1lGAEgASgJEgwKBGhvc3QYAiABKAkSDAoEdXNlchgDIAEoCRIMCgRwb3J0GAQgASgFEhEKCWJhc2VfcGF0aBgFIAEoCRIUCgxoYXNfaWRlbnRpdHkYBiABKAgiSwoRRHJhZnRSZW1vdGVUYXJnZXQSDAoEbmFtZRgBIAEoCRIMCgRob3N0GAIgASgJEgwKBHVzZXIYAyABKAkSDAoEcG9ydBgEIAEoBSJgChtUZXN0UmVtb3RlQ29ubmVjdGlvblJlcXVlc3QSEwoLcmVtb3RlX25hbWUYASABKAkSLAoFZHJhZnQYBSABKAsyHS5zZXNzaW9uLnYxLkRyYWZ0UmVtb3RlVGFyZ2V0InUKHFRlc3RSZW1vdGVDb25uZWN0aW9uUmVzcG9uc2USDwoHc3VjY2VzcxgBIAEoCBIYChBob3N0X2tleV91bmtub3duGAIgASgIEhMKC2ZpbmdlcnByaW50GAMgASgJEhUKDWVycm9yX21lc3NhZ2UYBCABKAkicwoZVHJ1c3RSZW1vdGVIb3N0S2V5UmVxdWVzdBITCgtyZW1vdGVfbmFtZRgBIAEoCRITCgtmaW5nZXJwcmludBgCIAEoCRIsCgVkcmFmdBgDIAEoCzIdLnNlc3Npb24udjEuRHJhZnRSZW1vdGVUYXJnZXQiRAoaVHJ1c3RSZW1vdGVIb3N0S2V5UmVzcG9uc2USDwoHc3VjY2VzcxgBIAEoCBIVCg1lcnJvcl9tZXNzYWdlGAIgASgJIi0KHUdlbmVyYXRlUmVtb3RlSWRlbnRpdHlSZXF1ZXN0EgwKBG5hbWUYASABKAkiVwoeR2VuZXJhdGVSZW1vdGVJZGVudGl0eVJlc3BvbnNlEhcKD3B1YmxpY19rZXlfdGV4dBgBIAEoCRIcChRhdXRob3JpemVkX2tleXNfbGluZRgCIAEoCSIUChJMaXN0UmVtb3Rlc1JlcXVlc3QiRQoTTGlzdFJlbW90ZXNSZXNwb25zZRIuCgdyZW1vdGVzGAEgAygLMh0uc2Vzc2lvbi52MS5SZW1vdGVDb25maWdQcm90byJgChNDcmVhdGVSZW1vdGVSZXF1ZXN0EgwKBG5hbWUYASABKAkSDAoEaG9zdBgCIAEoCRIMCgR1c2VyGAMgASgJEgwKBHBvcnQYBCABKAUSEQoJYmFzZV9wYXRoGAUgASgJIkUKFENyZWF0ZVJlbW90ZVJlc3BvbnNlEi0KBnJlbW90ZRgBIAEoCzIdLnNlc3Npb24udjEuUmVtb3RlQ29uZmlnUHJvdG8iIwoTRGVsZXRlUmVtb3RlUmVxdWVzdBIMCgRuYW1lGAEgASgJIhYKFERlbGV0ZVJlbW90ZVJlc3BvbnNlKrsBChVSZW1vdGVDb25uZWN0aW9uU3RhdGUSJwojUkVNT1RFX0NPTk5FQ1RJT05fU1RBVEVfVU5TUEVDSUZJRUQQABIlCiFSRU1PVEVfQ09OTkVDVElPTl9TVEFURV9DT05ORUNURUQQARIoCiRSRU1PVEVfQ09OTkVDVElPTl9TVEFURV9SRUNPTk5FQ1RJTkcQAhIoCiRSRU1PVEVfQ09OTkVDVElPTl9TVEFURV9ESVNDT05ORUNURUQQAzLSBAoNUmVtb3RlU2VydmljZRJrChRUZXN0UmVtb3RlQ29ubmVjdGlvbhInLnNlc3Npb24udjEuVGVzdFJlbW90ZUNvbm5lY3Rpb25SZXF1ZXN0Giguc2Vzc2lvbi52MS5UZXN0UmVtb3RlQ29ubmVjdGlvblJlc3BvbnNlIgASZQoSVHJ1c3RSZW1vdGVIb3N0S2V5EiUuc2Vzc2lvbi52MS5UcnVzdFJlbW90ZUhvc3RLZXlSZXF1ZXN0GiYuc2Vzc2lvbi52MS5UcnVzdFJlbW90ZUhvc3RLZXlSZXNwb25zZSIAEnEKFkdlbmVyYXRlUmVtb3RlSWRlbnRpdHkSKS5zZXNzaW9uLnYxLkdlbmVyYXRlUmVtb3RlSWRlbnRpdHlSZXF1ZXN0Giouc2Vzc2lvbi52MS5HZW5lcmF0ZVJlbW90ZUlkZW50aXR5UmVzcG9uc2UiABJQCgtMaXN0UmVtb3RlcxIeLnNlc3Npb24udjEuTGlzdFJlbW90ZXNSZXF1ZXN0Gh8uc2Vzc2lvbi52MS5MaXN0UmVtb3Rlc1Jlc3BvbnNlIgASUwoMQ3JlYXRlUmVtb3RlEh8uc2Vzc2lvbi52MS5DcmVhdGVSZW1vdGVSZXF1ZXN0GiAuc2Vzc2lvbi52MS5DcmVhdGVSZW1vdGVSZXNwb25zZSIAElMKDERlbGV0ZVJlbW90ZRIfLnNlc3Npb24udjEuRGVsZXRlUmVtb3RlUmVxdWVzdBogLnNlc3Npb24udjEuRGVsZXRlUmVtb3RlUmVzcG9uc2UiAEKrAQoOY29tLnNlc3Npb24udjFCC1JlbW90ZVByb3RvUAFaQ2dpdGh1Yi5jb20vdHN0YXBsZXIvc3RhcGxlci1zcXVhZC9nZW4vcHJvdG8vZ28vc2Vzc2lvbi92MTtzZXNzaW9udjGiAgNTWFiqAgpTZXNzaW9uLlYxygIKU2Vzc2lvblxWMeICFlNlc3Npb25cVjFcR1BCTWV0YWRhdGHqAgtTZXNzaW9uOjpWMWIGcHJvdG8z");
/**
 * Describes the message session.v1.RemoteConfigProto.
 * Use `create(RemoteConfigProtoSchema)` to create a new message.
 */ const RemoteConfigProtoSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_remote, 0)));
/**
 * Describes the message session.v1.DraftRemoteTarget.
 * Use `create(DraftRemoteTargetSchema)` to create a new message.
 */ const DraftRemoteTargetSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_1__/* .messageDesc */ .Z)(file_session_v1_remote, 1);
/**
 * Describes the message session.v1.TestRemoteConnectionRequest.
 * Use `create(TestRemoteConnectionRequestSchema)` to create a new message.
 */ const TestRemoteConnectionRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_1__/* .messageDesc */ .Z)(file_session_v1_remote, 2);
/**
 * Describes the message session.v1.TestRemoteConnectionResponse.
 * Use `create(TestRemoteConnectionResponseSchema)` to create a new message.
 */ const TestRemoteConnectionResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_remote, 3)));
/**
 * Describes the message session.v1.TrustRemoteHostKeyRequest.
 * Use `create(TrustRemoteHostKeyRequestSchema)` to create a new message.
 */ const TrustRemoteHostKeyRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_1__/* .messageDesc */ .Z)(file_session_v1_remote, 4);
/**
 * Describes the message session.v1.TrustRemoteHostKeyResponse.
 * Use `create(TrustRemoteHostKeyResponseSchema)` to create a new message.
 */ const TrustRemoteHostKeyResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_remote, 5)));
/**
 * Describes the message session.v1.GenerateRemoteIdentityRequest.
 * Use `create(GenerateRemoteIdentityRequestSchema)` to create a new message.
 */ const GenerateRemoteIdentityRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_1__/* .messageDesc */ .Z)(file_session_v1_remote, 6);
/**
 * Describes the message session.v1.GenerateRemoteIdentityResponse.
 * Use `create(GenerateRemoteIdentityResponseSchema)` to create a new message.
 */ const GenerateRemoteIdentityResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_remote, 7)));
/**
 * Describes the message session.v1.ListRemotesRequest.
 * Use `create(ListRemotesRequestSchema)` to create a new message.
 */ const ListRemotesRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_1__/* .messageDesc */ .Z)(file_session_v1_remote, 8);
/**
 * Describes the message session.v1.ListRemotesResponse.
 * Use `create(ListRemotesResponseSchema)` to create a new message.
 */ const ListRemotesResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_remote, 9)));
/**
 * Describes the message session.v1.CreateRemoteRequest.
 * Use `create(CreateRemoteRequestSchema)` to create a new message.
 */ const CreateRemoteRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_1__/* .messageDesc */ .Z)(file_session_v1_remote, 10);
/**
 * Describes the message session.v1.CreateRemoteResponse.
 * Use `create(CreateRemoteResponseSchema)` to create a new message.
 */ const CreateRemoteResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_remote, 11)));
/**
 * Describes the message session.v1.DeleteRemoteRequest.
 * Use `create(DeleteRemoteRequestSchema)` to create a new message.
 */ const DeleteRemoteRequestSchema = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_1__/* .messageDesc */ .Z)(file_session_v1_remote, 12);
/**
 * Describes the message session.v1.DeleteRemoteResponse.
 * Use `create(DeleteRemoteResponseSchema)` to create a new message.
 */ const DeleteRemoteResponseSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_session_v1_remote, 13)));
/**
 * RemoteConnectionState mirrors session/sshremote.RemoteConnectionState for
 * wire transport (Epic 6.2's RemoteHealthChangedEvent). Kept as a proto enum
 * rather than a bare string so frontend switch/exhaustiveness checks catch a
 * state the client doesn't yet handle, instead of silently falling through
 * on an unrecognized string value.
 *
 * @generated from enum session.v1.RemoteConnectionState
 */ var RemoteConnectionState = /*#__PURE__*/ function(RemoteConnectionState) {
    /**
   * @generated from enum value: REMOTE_CONNECTION_STATE_UNSPECIFIED = 0;
   */ RemoteConnectionState[RemoteConnectionState["UNSPECIFIED"] = 0] = "UNSPECIFIED";
    /**
   * @generated from enum value: REMOTE_CONNECTION_STATE_CONNECTED = 1;
   */ RemoteConnectionState[RemoteConnectionState["CONNECTED"] = 1] = "CONNECTED";
    /**
   * @generated from enum value: REMOTE_CONNECTION_STATE_RECONNECTING = 2;
   */ RemoteConnectionState[RemoteConnectionState["RECONNECTING"] = 2] = "RECONNECTING";
    /**
   * @generated from enum value: REMOTE_CONNECTION_STATE_DISCONNECTED = 3;
   */ RemoteConnectionState[RemoteConnectionState["DISCONNECTED"] = 3] = "DISCONNECTED";
    return RemoteConnectionState;
}({});
/**
 * Describes the enum session.v1.RemoteConnectionState.
 */ const RemoteConnectionStateSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (enumDesc(file_session_v1_remote, 0)));
/**
 * RemoteService validates SSH connectivity to a configured remote host and
 * drives the app's TOFU (trust-on-first-use) host-key confirmation flow
 * (ssh-remote-workspaces Epic 3.3). Kept as its own proto file, separate
 * from session.proto: a remote is a distinct resource, not an extension of
 * CreateSessionRequest.
 *
 * @generated from service session.v1.RemoteService
 */ const RemoteService = /*@__PURE__*/ (0,_bufbuild_protobuf_codegenv2__WEBPACK_IMPORTED_MODULE_2__/* .serviceDesc */ .Z)(file_session_v1_remote, 0);


/***/ }),

/***/ 90062:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   ZX: () => (/* binding */ createSessionWatchTransport)
/* harmony export */ });
/* unused harmony exports fromWebSocket, createWatchTransport */
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(45640);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(16294);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_6__ = __webpack_require__(13399);
/* harmony import */ var _connectrpc_connect_web__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(46564);
/* harmony import */ var _connectrpc_connect_protocol__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(70050);
/* harmony import */ var _connectrpc_connect_protocol__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(69522);
/* harmony import */ var _connectrpc_connect_protocol__WEBPACK_IMPORTED_MODULE_5__ = __webpack_require__(9103);
/* harmony import */ var _connectrpc_connect_protocol__WEBPACK_IMPORTED_MODULE_7__ = __webpack_require__(48210);
/* harmony import */ var _connectrpc_connect_protocol_connect__WEBPACK_IMPORTED_MODULE_8__ = __webpack_require__(60713);
/* provided dependency */ var process = __webpack_require__(36843);
/* __next_internal_client_entry_do_not_use__ fromWebSocket,createWatchTransport,createSessionWatchTransport auto */ 



function encodeEnvelope(flags, data) {
    const buf = new Uint8Array(5 + data.length);
    buf[0] = flags;
    new DataView(buf.buffer).setUint32(1, data.length, false);
    buf.set(data, 5);
    return buf;
}
async function* fromWebSocket(ws, signal) {
    const queue = [];
    let notify = null;
    const push = (item)=>{
        queue.push(item);
        notify === null || notify === void 0 ? void 0 : notify();
        notify = null;
    };
    ws.onmessage = (e)=>push(new Uint8Array(e.data));
    ws.onerror = ()=>push(new _connectrpc_connect__WEBPACK_IMPORTED_MODULE_0__/* .ConnectError */ .T("WebSocket error", _connectrpc_connect__WEBPACK_IMPORTED_MODULE_1__/* .Code */ .C.Unavailable));
    ws.onclose = (ev)=>{
        if ((signal === null || signal === void 0 ? void 0 : signal.aborted) || ev.code === 1000) {
            push(null); // intentional close (1000=normal) or signal abort
        } else {
            // Code 1001 (Going Away / server restart) sets wasClean=true via TCP FIN but IS retriable.
            // Only 1000 and explicit signal abort are treated as non-retriable clean closes.
            push(new _connectrpc_connect__WEBPACK_IMPORTED_MODULE_0__/* .ConnectError */ .T("WebSocket closed", _connectrpc_connect__WEBPACK_IMPORTED_MODULE_1__/* .Code */ .C.Unavailable, new Headers({
                "ws-close-code": String(ev.code)
            })));
        }
    };
    const abortHandler = ()=>{
        ws.close();
        push(null);
    };
    signal === null || signal === void 0 ? void 0 : signal.addEventListener("abort", abortHandler);
    try {
        while(true){
            while(queue.length === 0){
                await new Promise((r)=>{
                    notify = r;
                });
            }
            const item = queue.shift();
            if (item === null) return;
            if (item instanceof Error) throw item;
            yield item;
        }
    } finally{
        signal === null || signal === void 0 ? void 0 : signal.removeEventListener("abort", abortHandler);
    }
}
/**
 * Creates a ConnectRPC transport that uses standard HTTP fetch for unary calls
 * and WebSocket for server-streaming calls. Compatible with StreamingWSBridge on
 * the server (server/services/ws_stream_bridge.go).
 *
 * Protocol (streaming):
 *   Client → first binary WS message: Connect request envelope (5-byte header + proto)
 *   Server → N binary WS messages: Connect response envelopes
 *   Server → final binary WS message: Connect end-stream envelope
 *
 * Auth: cookie-based (browser sends cookies automatically on WS upgrade).
 */ function createWatchTransport(opt) {
    const httpTransport = (0,_connectrpc_connect_web__WEBPACK_IMPORTED_MODULE_2__/* .createConnectTransport */ .w)(opt);
    return {
        unary: httpTransport.unary.bind(httpTransport),
        async stream (method, signal, timeoutMs, header, input, contextValues) {
            var _opt_useBinaryFormat;
            const useBinaryFormat = (_opt_useBinaryFormat = opt.useBinaryFormat) !== null && _opt_useBinaryFormat !== void 0 ? _opt_useBinaryFormat : true;
            const { serialize, parse } = (0,_connectrpc_connect_protocol__WEBPACK_IMPORTED_MODULE_3__/* .createClientMethodSerializers */ .Ef)(method, useBinaryFormat, opt.jsonOptions, opt.binaryOptions);
            const methodUrl = (0,_connectrpc_connect_protocol__WEBPACK_IMPORTED_MODULE_4__/* .createMethodUrl */ .j)(opt.baseUrl, method);
            return (0,_connectrpc_connect_protocol__WEBPACK_IMPORTED_MODULE_5__/* .runStreamingCall */ .u)({
                interceptors: opt.interceptors,
                timeoutMs,
                signal,
                req: {
                    stream: true,
                    service: method.parent,
                    method,
                    url: methodUrl,
                    requestMethod: "POST",
                    header: new Headers(header instanceof Headers ? header : undefined),
                    contextValues: contextValues !== null && contextValues !== void 0 ? contextValues : (0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_6__/* .createContextValues */ .k)(),
                    message: input
                },
                next: async (req)=>{
                    const wsUrl = req.url.replace(/^http/, "ws");
                    const ws = new WebSocket(wsUrl);
                    ws.binaryType = "arraybuffer";
                    await new Promise((resolve, reject)=>{
                        ws.onopen = ()=>resolve();
                        ws.onerror = ()=>reject(new _connectrpc_connect__WEBPACK_IMPORTED_MODULE_0__/* .ConnectError */ .T("WebSocket connection failed", _connectrpc_connect__WEBPACK_IMPORTED_MODULE_1__/* .Code */ .C.Unavailable));
                    });
                    // Read the first (and only) request message and send it as a Connect envelope.
                    const iter = req.message[Symbol.asyncIterator]();
                    const first = await iter.next();
                    if (first.done) {
                        ws.close();
                        throw new _connectrpc_connect__WEBPACK_IMPORTED_MODULE_0__/* .ConnectError */ .T("missing request message", _connectrpc_connect__WEBPACK_IMPORTED_MODULE_1__/* .Code */ .C.Internal);
                    }
                    ws.send(encodeEnvelope(0, serialize(first.value)));
                    const trailer = new Headers();
                    async function* parseResponses() {
                        let buf = new Uint8Array(0);
                        let endStreamReceived = false;
                        for await (const chunk of fromWebSocket(ws, signal)){
                            // Accumulate chunk into buffer
                            const merged = new Uint8Array(buf.length + chunk.length);
                            merged.set(buf);
                            merged.set(chunk, buf.length);
                            buf = merged;
                            // Parse all complete envelopes
                            while(buf.length >= 5){
                                const flags = buf[0];
                                const msgLen = new DataView(buf.buffer, buf.byteOffset).getUint32(1, false);
                                if (buf.length < 5 + msgLen) break;
                                const data = buf.slice(5, 5 + msgLen);
                                buf = buf.slice(5 + msgLen);
                                if (flags & _connectrpc_connect_protocol__WEBPACK_IMPORTED_MODULE_7__/* .compressedFlag */ .y) {
                                    throw new _connectrpc_connect__WEBPACK_IMPORTED_MODULE_0__/* .ConnectError */ .T("unexpected compressed response", _connectrpc_connect__WEBPACK_IMPORTED_MODULE_1__/* .Code */ .C.Internal);
                                }
                                if (flags & _connectrpc_connect_protocol_connect__WEBPACK_IMPORTED_MODULE_8__/* .endStreamFlag */ .Cu) {
                                    endStreamReceived = true;
                                    const end = (0,_connectrpc_connect_protocol_connect__WEBPACK_IMPORTED_MODULE_8__/* .endStreamFromJson */ .mo)(data);
                                    if (end.error) throw end.error;
                                    end.metadata.forEach((v, k)=>trailer.set(k, v));
                                } else {
                                    yield parse(data);
                                }
                            }
                        }
                        if (!endStreamReceived) {
                            throw new _connectrpc_connect__WEBPACK_IMPORTED_MODULE_0__/* .ConnectError */ .T("stream ended without end-stream message", _connectrpc_connect__WEBPACK_IMPORTED_MODULE_1__/* .Code */ .C.Internal);
                        }
                    }
                    return {
                        ...req,
                        header: new Headers({
                            "Content-Type": "application/connect+proto"
                        }),
                        trailer,
                        message: parseResponses()
                    };
                }
            });
        }
    };
}
/**
 * Selects the transport for any server-streaming Watch* RPC (SessionService,
 * ReviewQueueService, BacklogService, InsightsService,
 * UnfinishedWorkService, ...) whose path is registered with server.go's
 * StreamingWSBridge.
 *
 * Native ConnectRPC streaming only helps over a real HTTP/2 connection —
 * that's the TLS remote-access listener (:8444) only. No shipping browser
 * supports cleartext HTTP/2, so on the plain :8543 dev listener native
 * streaming would silently stay on HTTP/1.1 and hold a long-lived fetch()
 * open, reintroducing the exact browser connection-slot problem
 * StreamingWSBridge exists to avoid. Gate on baseUrl, not just the flag.
 */ function createSessionWatchTransport(opt) {
    const nativeStreamingEnabled = process.env.NEXT_PUBLIC_CONNECTRPC_NATIVE_STREAMING === "true";
    if (nativeStreamingEnabled && opt.baseUrl.startsWith("https://")) {
        return (0,_connectrpc_connect_web__WEBPACK_IMPORTED_MODULE_2__/* .createConnectTransport */ .w)(opt);
    }
    return createWatchTransport(opt);
}


/***/ }),

/***/ 90814:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   O: () => (/* binding */ usePageView)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* harmony import */ var _lib_nav__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(41078);
/* harmony import */ var _lib_contexts_AnalyticsContext__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(24044);
/* __next_internal_client_entry_do_not_use__ usePageView auto */ 


function usePageView() {
    const pathname = (0,_lib_nav__WEBPACK_IMPORTED_MODULE_1__/* .usePathname */ .a8)();
    const { track } = (0,_lib_contexts_AnalyticsContext__WEBPACK_IMPORTED_MODULE_2__/* .useAnalytics */ .st)();
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        track({
            name: "page_view",
            category: "navigation",
            page: pathname
        });
    }, [
        pathname,
        track
    ]);
}


/***/ }),

/***/ 99863:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  Link: () => (/* binding */ Link),
  Redirect: () => (/* binding */ Redirect),
  pushHistoryMarker: () => (/* binding */ pushHistoryMarker),
  pushTrayEntry: () => (/* binding */ pushTrayEntry),
  replaceHash: () => (/* binding */ replaceHash),
  usePathname: () => (/* binding */ usePathname),
  useReplaceSearch: () => (/* binding */ useReplaceSearch),
  useRouter: () => (/* binding */ useRouter),
  useSearchParams: () => (/* binding */ useSearchParams)
});

// UNUSED EXPORTS: nextImpl

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/client/app-dir/link.js
var app_dir_link = __webpack_require__(6024);
var link_default = /*#__PURE__*/__webpack_require__.n(app_dir_link);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/api/navigation.js
var navigation = __webpack_require__(45853);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
;// ./src/lib/nav/history.ts
// The only place outside an impl that touches raw History; every impl shares it so foreign state survives the same way.
const TRAY_MARKER = "ssqTray";
function replaceSearchInHistory(search) {
    const query = search === "" || search.startsWith("?") ? search : "?".concat(search);
    window.history.replaceState(window.history.state, "", query === "" ? window.location.pathname : query);
}
function replaceHashInHistory(hash) {
    const url = hash ? "".concat(window.location.pathname).concat(window.location.search).concat(hash.startsWith("#") ? hash : "#".concat(hash)) : window.location.pathname + window.location.search;
    window.history.replaceState(window.history.state, "", url);
}
function pushHistoryMarkerInHistory(key, value) {
    let replace = arguments.length > 2 && arguments[2] !== void 0 ? arguments[2] : false;
    var _window_history_state;
    const state = {
        ...(_window_history_state = window.history.state) !== null && _window_history_state !== void 0 ? _window_history_state : {},
        [key]: value
    };
    if (replace) window.history.replaceState(state, "");
    else window.history.pushState(state, "");
}
function pushTrayEntryInHistory() {
    let replace = arguments.length > 0 && arguments[0] !== void 0 ? arguments[0] : false;
    pushHistoryMarkerInHistory(TRAY_MARKER, true, replace);
}

;// ./src/lib/nav/normalize.ts
/** Strips trailing slashes (Next's `trailingSlash` export reports `/x/`) but keeps the root `/`. */ function normalizePathname(pathname) {
    const trimmed = pathname.replace(/\/+$/, "");
    return trimmed === "" ? "/" : trimmed;
}

;// ./src/lib/nav/impl.next.tsx
/* __next_internal_client_entry_do_not_use__ useRouter,useSearchParams,usePathname,Link,Redirect,useReplaceSearch,replaceHash,pushTrayEntry,pushHistoryMarker,nextImpl auto */ 





const toNextOptions = (options)=>(options === null || options === void 0 ? void 0 : options.resetScroll) === false ? {
        scroll: false
    } : undefined;
function useRouter() {
    const router = (0,navigation.useRouter)();
    return (0,react.useMemo)(()=>({
            push: (href, options)=>router.push(href, toNextOptions(options)),
            replace: (href, options)=>router.replace(href, toNextOptions(options)),
            back: ()=>router.back(),
            forward: ()=>router.forward()
        }), [
        router
    ]);
}
function useSearchParams() {
    return (0,navigation.useSearchParams)();
}
function usePathname() {
    var _useNextPathname;
    return normalizePathname((_useNextPathname = (0,navigation.usePathname)()) !== null && _useNextPathname !== void 0 ? _useNextPathname : "/");
}
/** Prefetch stays off: it triggers CSS preload warnings (the reason the NavLink this replaces existed). */ const Link = /*#__PURE__*/ (0,react.forwardRef)(function Link(param, ref) {
    let { resetScroll, ...props } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)((link_default()), {
        ref: ref,
        prefetch: false,
        scroll: resetScroll === false ? false : undefined,
        ...props
    });
});
/** Next's render-time `redirect()`: the behaviour the two redirect-only pages have today. */ function Redirect(param) {
    let { to } = param;
    (0,navigation.redirect)(to);
}
function useReplaceSearch() {
    return (0,react.useCallback)((search)=>replaceSearchInHistory(search), []);
}
const replaceHash = replaceHashInHistory;
const pushTrayEntry = pushTrayEntryInHistory;
const pushHistoryMarker = pushHistoryMarkerInHistory;
const nextImpl = {
    useRouter,
    useSearchParams,
    usePathname,
    Link,
    Redirect,
    useReplaceSearch,
    replaceHash,
    pushTrayEntry,
    pushHistoryMarker
};


/***/ })

},
/******/ __webpack_require__ => { // webpackRuntimeModules
/******/ var __webpack_exec__ = (moduleId) => (__webpack_require__(__webpack_require__.s = moduleId))
/******/ __webpack_require__.O(0, [1869,852,4202,9353,9691,3516,7358], () => (__webpack_exec__(52135)));
/******/ var __webpack_exports__ = __webpack_require__.O();
/******/ _N_E = __webpack_exports__;
/******/ }
]);
//# sourceMappingURL=page-b16776267020d096.js.map