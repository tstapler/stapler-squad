"use strict";
(self["webpackChunk_N_E"] = self["webpackChunk_N_E"] || []).push([[7461],{

/***/ 4942:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   T: () => (/* binding */ commandBarController)
/* harmony export */ });
/**
 * Imperative handle to the mounted command bar, so non-React callers (the
 * Alt+W alias, the Settings "Open command bar" button) can open it without
 * prop-drilling. `open` returns false when no bar is mounted or a modal dialog
 * is already open, letting callers fall back to their own behaviour.
 */ let opener = null;
const commandBarController = {
    attach (fn) {
        opener = fn;
        return ()=>{
            if (opener === fn) opener = null;
        };
    },
    open () {
        let path = arguments.length > 0 && arguments[0] !== void 0 ? arguments[0] : [];
        return opener ? opener(path) : false;
    }
};


/***/ }),

/***/ 7038:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  X: () => (/* binding */ OnboardingProvider),
  w: () => (/* binding */ useOnboardingContext)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
;// ./src/components/onboarding/useOnboarding.ts

const ONBOARDED_KEY = "stapler-squad:onboarded";
function useOnboarding() {
    const [showOnboarding, setShow] = (0,react.useState)(false);
    (0,react.useEffect)(()=>{
        let timerId;
        try {
            if (!localStorage.getItem(ONBOARDED_KEY)) {
                timerId = setTimeout(()=>setShow(true), 800);
            }
        } catch (e) {
        // ignore storage errors (private browsing mode, etc.)
        }
        return ()=>clearTimeout(timerId);
    }, []);
    const setOnboardingComplete = (0,react.useCallback)(()=>{
        try {
            localStorage.setItem(ONBOARDED_KEY, "true");
        } catch (e) {
        // ignore storage errors
        }
        setShow(false);
    }, []);
    const resetOnboarding = (0,react.useCallback)(()=>{
        try {
            localStorage.removeItem(ONBOARDED_KEY);
        } catch (e) {
        // ignore storage errors
        }
        setShow(true);
    }, []);
    return {
        showOnboarding,
        setOnboardingComplete,
        resetOnboarding
    };
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@radix-ui+react-dialog@1.1.15+b721ee6ccd61acaa/node_modules/@radix-ui/react-dialog/dist/index.mjs + 25 modules
var dist = __webpack_require__(54069);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/promise-client.js + 2 modules
var promise_client = __webpack_require__(66144);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect-web@2.1.1+0149f91553ca585e/node_modules/@connectrpc/connect-web/dist/esm/connect-transport.js + 8 modules
var connect_transport = __webpack_require__(46564);
// EXTERNAL MODULE: ./src/gen/session/v1/session_pb.ts
var session_pb = __webpack_require__(23575);
// EXTERNAL MODULE: ./src/lib/config.ts
var config = __webpack_require__(51202);
// EXTERNAL MODULE: ./src/lib/contexts/OmnibarContext.tsx + 43 modules
var OmnibarContext = __webpack_require__(40218);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fui%2FModalTour.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA81W226jMBB9z1fMy0qNtESQbG70pX-yMjCAW2OztgmBVf99hSGUcAlUXa32pcKe8VzOnDnpyxsWoSQJKoiP2vaYDb9XAKEUifkAECnxqS5csJ9XAO8rAC36Jqc2va9e-uGcx-EAtCRchUImLiifMHyyN-fjeibVyDNnfathU2fempepUFRTwV0I6RWDyodyhbrJ7xH_LZIi44ELFyKfLOunc8ZyK0ti4gVUpYwULoQMr9UFYTTiFtWYKBd85Bpldf2aKU3DwvIF18h111RalAd4dcGxbZOTcJqQuqYb5Pepd2INoZA5kYF6_mhoN9WQFqkLe_tb9c0w1O2hA5L5ZETjk7W3v32H6u_6MQLn2i5kgNIFJ72CEowGPa_XjpclSUAz1Q-0OxmflAQB5dHAujXWhFytnAY6dmG_tVMDdnM-25f85hIjjWLtwml_ias7cUEZMpFbhQsk02IZ4s4yxH8YxJdyICJpv7dt0fQmI8otT2gtkkH_FXPblHuTsmn8UMNw67k5Psb6PDNVXLfMaHjUC8C75RxMOdPRkq7zcYFzVwA2-87jU4_dxFOCZRpbgo-gBiBrZEZtoeDaUrTEgd0zdl8wIQcTG8DHBcfuItzOfiZV9T4V9MaACYZvi3X_plywNYdPTerkxtUuGBRHO7NniHHqRjvXsj2JIH4gnOPoDOg0xvb8UjQANYBa42PedSsmMxU_mHlNF0Y5tvLibA6fXFzvXisoN_E8Jvy3O2pUOjoqEtOlkw97SBLKigFa2RzY02M_LlJ5XMzXPKYaLZUSH6tdySVJOyj5X1bUcqGijixoW0XwVa5Mh8ZPNTj4t8HgZnmoc0S-CIIpbWwrCv8K5G24qNbpCaUr-0q3UIsfKUk8PY1fM-RO7sh9r-RTND6OT3i5EkcdJZ4urfeE-JpecOaN6L6J__kgov9BZb48nnjReE7dUkZbrn7g3v8AdZCtfz8NAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fui_2FModalTour_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA81W226jMBB9z1fMy0qNtESQbG70pX_yMjCAW2OztgmBVf99hSGUcAlUXa32pcKe8VzOnDnpyxsWoSQJKoiP2vaYDb9XAKEUifkAECnxqS5csJ9XAO8rAC36Jqc2va9e_uGcx_EAtCRchUImLiifMHyyN_fjeibVyDNnfathU2fempepUFRTwV0I6RWDyodyhbrJ7xH_LZIi44ELFyKfLOunc8ZyK0ti4gVUpYwULoQMr9UFYTTiFtWYKBd85Bpldf2aKU3DwvIF18h111RalAd4dcGxbZOTcJqQuqYb5Pepd2INoZA5kYF6_mhoN9WQFqkLe_tb9c0w1O2hA5L5ZETjk7W3v32H6u_6MQLn2i5kgNIFJ72CEowGPa_XjpclSUAz1Q_0OxmflAQB5dHAujXWhFytnAY6dmG_tVMDdnM_25f85hIjjWLtwml_ias7cUEZMpFbhQsk02IZ4s4yxH8YxJdyICJpv7dt0fQmI8otT2gtkkH_FXPblHuTsmn8UMNw67k5Psb6PDNVXLfMaHjUC8C75RxMOdPRkq7zcYFzVwA2_87jU4_dxFOCZRpbgo_gBiBrZEZtoeDaUrTEgd0zdl8wIQcTG8DHBcfuItzOfiZV9T4V9MaACYZvi3X_plywNYdPTerkxtUuGBRHO7NniHHqRjvXsj2JIH4gnOPoDOg0xvb8UjQANYBa42PedSsmMxU_mHlNF0Y5tvLibA6fXFzvXisoN_E8Jvy3O2pUOjoqEtOlkw97SBLKigFa2RzY02M_LlJ5XMzXPKYaLZUSH6tdySVJOyj5X1bUcqGijixoW0XwVa5Mh8ZPNTj4t8HgZnmoc0S_CIIpbWwrCv8K5G24qNbpCaUr_0q3UIsfKUk8PY1fM_RO7sh9r_RTND6OT3i5EkcdJZ4urfeE_JpecOaN6L6J_kgov9BZb48nnjReE7dUkZbrn7g3v8AdZCtfz8NAAA_22_7D = __webpack_require__(60160);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fonboarding%2FOnboardingModal.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA5VSy27CMBC88xV7qVQORknoQzUfg4y9SbZ17Mje8Kr67xVGEWAobW-JZ2Y9O57ZsvS6rV63BXxOAAzF3qqdhNridjEBUJYaJ4ixi8dDEVkFPkDvQ2Sqd0J7x-hYQuyVRrFC3iC6A6NToSEnVp7ZdxLWKjwKsSzfcF-F_bycLiZfk9looEwGau9Y1Koju8sF1TBdjIxIe7waqBKuvfUhx8oE9coYcs2VsEjoSumPJvjBmZzwesR9MBhEUIaGeDXjJXE2LTGKFISEPuDfUgCw5FC0SE3LEsrZ8-k-CWW_hegtmUyKl_lVKb8NGW4llEXxcOZZe2tVH1HC-PXv55nf7sfvJbhokEbHGO68RbWbQnHmfDT3Uwh15lJaFVnolqxJhrMxzju8EDydWnezU6v7nTqb9Jwm3VkrO9n_0uY7N3dXfT3uderM-K-HEA_63tMYPOOWhUHtg2LyTsLgDIZDARMalIt0BDJDLttXtn6NIW1902XifwPjqri2YAQAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fonboarding_2FOnboardingModal_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA5VSy27CMBC88xV7qVQORknoQzUfg4y9SbZ17Mje8Kr67xVGEWAobW_JZ2Y9O57ZsvS6rV63BXxOAAzF3qqdhNridjEBUJYaJ4ixi8dDEVkFPkDvQ2Sqd0J7x_hYQuyVRrFC3iC6A6NToSEnVp7ZdxLWKjwKsSzfcF_F_bycLiZfk9looEwGau9Y1Koju8sF1TBdjIxIe7waqBKuvfUhx8oE9coYcs2VsEjoSumPJvjBmZzwesR9MBhEUIaGeDXjJXE2LTGKFISEPuDfUgCw5FC0SE3LEsrZ8_k_CWW_hegtmUyKl_lVKb8NGW4llEXxcOZZe2tVH1HC_PXv55nf7sfvJbhokEbHGO68RbWbQnHmfDT3Uwh15lJaFVnolqxJhrMxzju8EDydWnezU6v7nTqb9Jwm3VkrO9n_0uY7N3dXfT3uderM_K_HEA_63tMYPOOWhUHtg2LyTsLgDIZDARMalIt0BDJDLttXtn6NIW1902XifwPjqri2YAQAAA_22_7D = __webpack_require__(42703);
;// ./src/components/onboarding/OnboardingModal.css.ts



var asciiDiagram = '_1och27x1';
var body = 'h7t0bla';
var checkboxLabel = 'h7t0bld';
var checkboxRow = 'h7t0blc';
var content = 'h7t0bl3';
var dot = 'h7t0bl5';
var dotActive = 'h7t0bl6';
var dotCompleted = 'h7t0bl7';
var footer = 'h7t0ble';
var footerRight = 'h7t0blf';
var header = '_1och27x0';
var headline = 'h7t0bl9';
var kbd = 'h7t0blb';
var linkButton = '_1och27x5';
var overlay = 'h7t0bl2';
var primaryButton = 'h7t0blg';
var secondaryButton = 'h7t0blh';
var shortcutLabel = '_1och27x4';
var shortcutRow = '_1och27x3';
var shortcutTable = '_1och27x2';
var skipButton = 'h7t0bl8';
var stepIndicatorRow = 'h7t0bl4';
;// ./src/components/onboarding/OnboardingModal.tsx
// +feature: onboarding-hook-install
/* __next_internal_client_entry_do_not_use__ OnboardingModal auto */ 









const SHORTCUTS = [
    {
        key: "⌘K / Ctrl+K",
        label: "Open omnibar"
    },
    {
        key: "?",
        label: "Shortcut cheatsheet"
    },
    {
        key: "[",
        label: "Toggle nav"
    },
    {
        key: "⌘P / Ctrl+P",
        label: "Pause session"
    },
    {
        key: "⌘D / Ctrl+D",
        label: "Delete session"
    },
    {
        key: "⌘↵ / Ctrl+↵",
        label: "Accept approval"
    }
];
const ASCII_DIAGRAM = "main ─┬─► worktree-A  (Claude)\n      │\n      └─► worktree-B  (Aider)";
const LAST_STEP = 5;
function StepIndicator(param) {
    let { current, total } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
        className: stepIndicatorRow,
        children: Array.from({
            length: total
        }, (_, i)=>{
            const stepNum = i + 1;
            let dotClass = dot;
            if (stepNum < current) dotClass = "".concat(dot, " ").concat(dotCompleted);
            if (stepNum === current) dotClass = "".concat(dot, " ").concat(dotActive);
            return /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: dotClass,
                "aria-hidden": "true"
            }, stepNum);
        })
    });
}
function OnboardingModal(param) {
    let { isOpen, onClose } = param;
    const [step, setStep] = (0,react.useState)(1);
    const [dontShowAgain, setDontShowAgain] = (0,react.useState)(true);
    const { open: openOmnibar } = (0,OmnibarContext/* useOmnibar */.W)();
    // Hook installation step state.
    const [installRules, setInstallRules] = (0,react.useState)(true);
    const [installNotifications, setInstallNotifications] = (0,react.useState)(true);
    const [rulesAvailable, setRulesAvailable] = (0,react.useState)(true);
    const [notificationsAvailable, setNotificationsAvailable] = (0,react.useState)(true);
    const [rulesInstalled, setRulesInstalled] = (0,react.useState)(false);
    const [notificationsInstalled, setNotificationsInstalled] = (0,react.useState)(false);
    const [hookBusy, setHookBusy] = (0,react.useState)(false);
    const [hookMessage, setHookMessage] = (0,react.useState)(null);
    const client = (0,react.useMemo)(()=>(0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, (0,connect_transport/* createConnectTransport */.w)({
            baseUrl: (0,config/* getApiBaseUrl */.h)()
        })), []);
    // Guards: avoid setState after unmount, and only seed the toggle defaults once
    // so navigating Back→forward to the hooks step doesn't clobber the user's edits.
    const mountedRef = (0,react.useRef)(true);
    const seededRef = (0,react.useRef)(false);
    (0,react.useEffect)(()=>{
        mountedRef.current = true;
        return ()=>{
            mountedRef.current = false;
        };
    }, []);
    const refreshHookStatus = (0,react.useCallback)(async ()=>{
        try {
            const res = await client.getHookStatus({});
            if (!mountedRef.current) return;
            setRulesInstalled(res.rulesInstalled);
            setNotificationsInstalled(res.notificationsInstalled);
            setRulesAvailable(res.rulesAvailable);
            setNotificationsAvailable(res.notificationsAvailable);
            // Pre-check a toggle only when the hook is available and not already installed.
            // Seed once so re-entering the step preserves any manual toggle changes.
            if (!seededRef.current) {
                seededRef.current = true;
                setInstallRules(res.rulesAvailable && !res.rulesInstalled);
                setInstallNotifications(res.notificationsAvailable && !res.notificationsInstalled);
            }
        } catch (e) {
            if (mountedRef.current) setHookMessage("Could not check hook status.");
        }
    }, [
        client
    ]);
    // Load current hook status when the user reaches the hooks step.
    (0,react.useEffect)(()=>{
        if (isOpen && step === 5) {
            void refreshHookStatus();
        }
    }, [
        isOpen,
        step,
        refreshHookStatus
    ]);
    const handleInstallHooks = async ()=>{
        setHookBusy(true);
        setHookMessage(null);
        try {
            const res = await client.installHooks({
                installRules,
                installNotifications
            });
            if (!mountedRef.current) return;
            if (res.status) {
                setRulesInstalled(res.status.rulesInstalled);
                setNotificationsInstalled(res.status.notificationsInstalled);
            }
            setHookMessage(res.messages.join(" ") || "Hooks updated.");
        } catch (e) {
            if (mountedRef.current) {
                setHookMessage("Failed to install hooks. You can set them up later from the docs.");
            }
        } finally{
            if (mountedRef.current) setHookBusy(false);
        }
    };
    const handleSkip = ()=>{
        try {
            localStorage.setItem("stapler-squad:onboarded", "true");
        } catch (e) {
        // ignore storage errors
        }
        onClose();
    };
    const handleNext = ()=>{
        if (step < LAST_STEP) {
            setStep((prev)=>prev + 1);
        }
    };
    const handleBack = ()=>{
        if (step > 1) {
            setStep((prev)=>prev - 1);
        }
    };
    const handleGetStarted = ()=>{
        if (dontShowAgain) {
            try {
                localStorage.setItem(ONBOARDED_KEY, "true");
            } catch (e) {
            // ignore storage errors
            }
        }
        onClose();
    };
    const handleTryOmnibar = ()=>{
        onClose();
        // Small delay to let modal close animation finish before opening omnibar
        setTimeout(()=>openOmnibar(), 100);
    };
    const handleViewShortcuts = ()=>{
        onClose();
        // Dispatch a custom event that CockpitShell listens for
        setTimeout(()=>{
            window.dispatchEvent(new CustomEvent("stapler-squad:open-shortcuts"));
        }, 100);
    };
    // Reset step when modal opens
    const handleOpenChange = (open)=>{
        if (open) {
            setStep(1);
            setDontShowAgain(true);
            // Re-seed toggle defaults from fresh status on a new onboarding run.
            seededRef.current = false;
            setHookMessage(null);
        }
        if (!open) {
            onClose();
        }
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(dist/* Root */.bL, {
        open: isOpen,
        onOpenChange: handleOpenChange,
        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)(dist/* Portal */.ZL, {
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)(dist/* Overlay */.hJ, {
                    className: overlay
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsxs)(dist/* Content */.UC, {
                    className: content,
                    "aria-describedby": "onboarding-step-description",
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)(dist/* Title */.hE, {
                            className: headline,
                            children: [
                                step === 1 && "One place for all your AI coding sessions",
                                step === 2 && "Each session is isolated",
                                step === 3 && "Create or navigate in one keystroke",
                                step === 4 && "Key shortcuts",
                                step === 5 && "Enable Claude Code hooks"
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)(StepIndicator, {
                            current: step,
                            total: 5
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            className: skipButton,
                            onClick: handleSkip,
                            "aria-label": "Skip onboarding",
                            children: "Skip"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            id: "onboarding-step-description",
                            children: [
                                step === 1 && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                            className: body,
                                            children: "stapler-squad runs each AI agent in an isolated tmux session so your agents never step on each other."
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("pre", {
                                            className: asciiDiagram,
                                            children: ASCII_DIAGRAM
                                        })
                                    ]
                                }),
                                step === 2 && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                    className: body,
                                    children: "Every session gets its own git worktree and directory. Agents write code in parallel without conflicts. Switch between sessions instantly — each one resumes exactly where it left off."
                                }),
                                step === 3 && /*#__PURE__*/ (0,jsx_runtime.jsx)(jsx_runtime.Fragment, {
                                    children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                                        className: body,
                                        children: [
                                            "Press ",
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("kbd", {
                                                className: kbd,
                                                children: "⌘K"
                                            }),
                                            " (or",
                                            " ",
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("kbd", {
                                                className: kbd,
                                                children: "Ctrl+K"
                                            }),
                                            ") to open the omnibar. Type a path, GitHub URL, or session name."
                                        ]
                                    })
                                }),
                                step === 4 && /*#__PURE__*/ (0,jsx_runtime.jsx)(jsx_runtime.Fragment, {
                                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                        className: shortcutTable,
                                        children: SHORTCUTS.map((param)=>{
                                            let { key, label } = param;
                                            return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                className: shortcutRow,
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: shortcutLabel,
                                                        children: label
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("kbd", {
                                                        className: kbd,
                                                        children: key
                                                    })
                                                ]
                                            }, label);
                                        })
                                    })
                                }),
                                step === 5 && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                            className: body,
                                            children: "Optionally hook stapler-squad into Claude Code so it works outside the app too. You can change this later."
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                            className: checkboxRow,
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                    type: "checkbox",
                                                    checked: installRules,
                                                    disabled: !rulesAvailable || rulesInstalled || hookBusy,
                                                    onChange: (e)=>setInstallRules(e.target.checked)
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                    className: checkboxLabel,
                                                    children: [
                                                        "Enable rule enforcement",
                                                        rulesInstalled ? " (already installed)" : !rulesAvailable ? " (ssq-hooks not installed — run `make install`)" : " — gate tool calls through your rules"
                                                    ]
                                                })
                                            ]
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                            className: checkboxRow,
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                    type: "checkbox",
                                                    checked: installNotifications,
                                                    disabled: !notificationsAvailable || notificationsInstalled || hookBusy,
                                                    onChange: (e)=>setInstallNotifications(e.target.checked)
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                    className: checkboxLabel,
                                                    children: [
                                                        "Enable notifications",
                                                        notificationsInstalled ? " (already installed)" : !notificationsAvailable ? " (ssq-hook-handler not found)" : " — chimes and alerts when Claude needs you"
                                                    ]
                                                })
                                            ]
                                        }),
                                        hookMessage && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                            className: body,
                                            children: hookMessage
                                        })
                                    ]
                                })
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: footer,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                    children: step > 1 && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        className: secondaryButton,
                                        onClick: handleBack,
                                        children: "Back"
                                    })
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: footerRight,
                                    children: [
                                        step === 3 && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                            className: secondaryButton,
                                            onClick: handleTryOmnibar,
                                            children: "Try it now"
                                        }),
                                        step === 4 && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                            className: linkButton,
                                            onClick: handleViewShortcuts,
                                            children: "View all shortcuts"
                                        }),
                                        step < LAST_STEP ? /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                            className: primaryButton,
                                            onClick: handleNext,
                                            children: "Next"
                                        }) : /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                                            children: [
                                                (installRules || installNotifications) && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                    className: secondaryButton,
                                                    onClick: handleInstallHooks,
                                                    disabled: hookBusy,
                                                    children: hookBusy ? "Installing…" : "Install"
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                                    className: checkboxRow,
                                                    children: [
                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                            type: "checkbox",
                                                            checked: dontShowAgain,
                                                            onChange: (e)=>setDontShowAgain(e.target.checked)
                                                        }),
                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                            className: checkboxLabel,
                                                            children: "Don't show this again"
                                                        })
                                                    ]
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                    className: primaryButton,
                                                    onClick: handleGetStarted,
                                                    children: "Get started"
                                                })
                                            ]
                                        })
                                    ]
                                })
                            ]
                        })
                    ]
                })
            ]
        })
    });
}

;// ./src/lib/contexts/OnboardingContext.tsx
/* __next_internal_client_entry_do_not_use__ useOnboardingContext,OnboardingProvider auto */ 



const OnboardingContext = /*#__PURE__*/ (0,react.createContext)(null);
function useOnboardingContext() {
    const context = (0,react.useContext)(OnboardingContext);
    if (!context) {
        throw new Error("useOnboardingContext must be used within an OnboardingProvider");
    }
    return context;
}
function OnboardingProvider(param) {
    let { children } = param;
    const { showOnboarding, setOnboardingComplete, resetOnboarding } = useOnboarding();
    const value = {
        triggerOnboarding: resetOnboarding
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(OnboardingContext.Provider, {
        value: value,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)(OnboardingModal, {
                isOpen: showOnboarding,
                onClose: setOnboardingComplete
            }),
            children
        ]
    });
}


/***/ }),

/***/ 8569:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   B6: () => (/* binding */ isInputElement),
/* harmony export */   u5: () => (/* binding */ registry)
/* harmony export */ });
/* unused harmony exports RESERVED_SEQUENCE_KEYS, ShortcutRegistry */
/** Keys the command bar reserves for list navigation; never valid in a sequence. */ const RESERVED_SEQUENCE_KEYS = new Set([
    "j",
    "k",
    "?"
]);
/**
 * ShortcutRegistry — centralized keyboard shortcut manager.
 *
 * - Single document.addEventListener("keydown") for all shortcuts.
 * - Context-sensitive: detects active context via data-context attribute
 *   on the focused element's ancestor chain.
 * - IME composition events are skipped for single-character shortcuts.
 * - Conflict detection: warns (does not throw) on duplicate id registration.
 */ class ShortcutRegistry {
    /**
   * Register a shortcut. Returns a cleanup function that deregisters it.
   */ register(id, shortcut) {
        if (shortcut.sequence) {
            if (shortcut.sequence.length === 0 || shortcut.sequence.some((k)=>k.length !== 1 || RESERVED_SEQUENCE_KEYS.has(k))) {
                console.warn('[ShortcutRegistry] Invalid sequence for "'.concat(id, '": ').concat(JSON.stringify(shortcut.sequence), " — ignoring sequence."));
                shortcut = {
                    ...shortcut,
                    sequence: undefined
                };
            }
        }
        if (this.shortcuts.has(id)) {
            console.warn('[ShortcutRegistry] Duplicate shortcut id: "'.concat(id, '" — overwriting.'));
        }
        this.shortcuts.set(id, shortcut);
        return ()=>{
            this.shortcuts.delete(id);
        };
    }
    /**
   * Get all registered shortcuts grouped by context.
   * Used by the ? overlay to render the shortcut reference.
   */ getAll() {
        const result = {
            global: [],
            "session-list": [],
            approval: [],
            terminal: [],
            cockpit: [],
            omnibar: []
        };
        for (const shortcut of this.shortcuts.values()){
            if (shortcut.hidden) continue;
            result[shortcut.context].push(shortcut);
        }
        return result;
    }
    /**
   * Every visible action, for the command bar's fuzzy palette: includes actions
   * with no sequence (they show their direct chord). Stable registration order.
   */ getPalette() {
        const out = [];
        for (const [id, shortcut] of this.shortcuts){
            if (!shortcut.hidden) out.push({
                id,
                shortcut
            });
        }
        return out;
    }
    /**
   * Group-nested which-key structure built from `sequence` fields. A node is
   * either a leaf (the sequence ends there) or a branch; if one registration's
   * sequence is a prefix of another's, the longer one wins the key and the
   * shorter is dropped (warned) so a key never both runs and descends.
   */ getTree() {
        const root = {
            key: "",
            label: "",
            children: []
        };
        // Longest sequences first so a branch always beats a leaf that is its prefix,
        // independent of React effect registration order.
        const entries = [
            ...this.shortcuts.entries()
        ].filter((param)=>{
            let [, sc] = param;
            return sc.sequence && !sc.hidden;
        }).sort((param, param1)=>{
            let [, a] = param, [, b] = param1;
            return b.sequence.length - a.sequence.length;
        });
        for (const [id, shortcut] of entries){
            const seq = shortcut.sequence;
            let node = root;
            let skipped = false;
            for(let i = 0; i < seq.length; i++){
                const key = seq[i];
                const isLast = i === seq.length - 1;
                let child = node.children.find((c)=>c.key === key);
                if (child === null || child === void 0 ? void 0 : child.shortcut) {
                    skipped = true; // would extend (or duplicate) a leaf
                    break;
                }
                if (!child) {
                    var _shortcut_group;
                    child = {
                        key,
                        label: i === 0 ? (_shortcut_group = shortcut.group) !== null && _shortcut_group !== void 0 ? _shortcut_group : key : key,
                        children: []
                    };
                    node.children.push(child);
                } else if (i === 0 && shortcut.group && child.label === child.key) {
                    child.label = shortcut.group;
                }
                if (isLast) {
                    if (child.children.length > 0) {
                        skipped = true; // a longer sequence already owns this key as a branch
                        break;
                    }
                    child.shortcut = shortcut;
                    child.label = shortcut.label;
                }
                node = child;
            }
            if (skipped) console.warn("[ShortcutRegistry] Sequence ".concat(JSON.stringify(seq), ' of "').concat(id, '" collides with another binding — skipped.'));
        }
        return root.children;
    }
    /**
   * Register a provider whose shortcuts exist only while the command bar is under
   * `prefix` (called by resolveLazy). Keeps per-item actions (e.g. reply options
   * 1..N) off the registry at mount.
   */ registerLazy(id, prefix, provider) {
        const entry = {
            prefix,
            provider,
            cleanups: []
        };
        this.lazyProviders.set(id, entry);
        return ()=>{
            entry.cleanups.forEach((c)=>c());
            this.lazyProviders.delete(id);
        };
    }
    /** Drop every lazily provided shortcut (the bar closed). */ releaseLazy() {
        for (const entry of this.lazyProviders.values()){
            entry.cleanups.forEach((c)=>c());
            entry.cleanups = [];
        }
    }
    /**
   * (Re)build lazily provided shortcuts whose prefix is compatible with `path`
   * (the path is a prefix of theirs, or theirs of it). With an empty path every
   * provider resolves, which the fuzzy palette needs. Returns when done; old
   * provided entries are replaced.
   */ resolveLazy(path) {
        for (const entry of this.lazyProviders.values()){
            const n = Math.min(path.length, entry.prefix.length);
            const compatible = path.slice(0, n).every((k, i)=>k === entry.prefix[i]);
            if (!compatible) continue;
            entry.cleanups.forEach((c)=>c());
            entry.cleanups = entry.provider().map((param)=>{
                let { id, shortcut } = param;
                return this.register(id, shortcut);
            });
        }
    }
    /**
   * Detect the active context by walking up the DOM from document.activeElement
   * looking for a [data-context] attribute.
   */ getActiveContext() {
        if (typeof document === "undefined") return "global";
        let el = document.activeElement;
        while(el){
            const ctx = el.getAttribute("data-context");
            if (ctx) return ctx;
            el = el.parentElement;
        }
        return "global";
    }
    dispatch(event) {
        // Skip IME composition for single-character shortcuts
        if (event.isComposing) return;
        // Skip if target is an input/textarea/select/contenteditable
        // (unless the shortcut explicitly uses a modifier)
        const hasModifier = event.metaKey || event.ctrlKey || event.altKey;
        if (!hasModifier && isInputElement(event.target)) return;
        const activeContext = this.getActiveContext();
        for (const shortcut of this.shortcuts.values()){
            if (shortcut.key === undefined && shortcut.code === undefined) continue;
            // Context check: global shortcuts fire everywhere (except inside terminal unless
            // the shortcut is terminal-scoped). Terminal-scoped shortcuts only fire in terminal.
            if (shortcut.context === "terminal" && activeContext !== "terminal") continue;
            if (activeContext === "terminal" && shortcut.context !== "terminal" && shortcut.context !== "global" && shortcut.context !== "cockpit") continue;
            if (shortcut.context !== "global" && shortcut.context !== activeContext) continue;
            if (!keyMatches(event, shortcut)) continue;
            event.preventDefault();
            shortcut.action();
            return; // first match wins
        }
    }
    /** Tear down the global listener (useful in tests). */ destroy() {
        if (typeof document !== "undefined") {
            document.removeEventListener("keydown", this.bound);
        }
    }
    constructor(){
        this.shortcuts = new Map();
        this.lazyProviders = new Map();
        this.bound = this.dispatch.bind(this);
        if (typeof document !== "undefined") {
            document.addEventListener("keydown", this.bound);
        }
    }
}
function keyMatches(event, shortcut) {
    const keyOk = shortcut.key !== undefined && event.key === shortcut.key;
    const codeOk = shortcut.code !== undefined && event.code === shortcut.code;
    if (!keyOk && !codeOk) return false;
    var _shortcut_modifiers;
    const m = (_shortcut_modifiers = shortcut.modifiers) !== null && _shortcut_modifiers !== void 0 ? _shortcut_modifiers : {};
    if (!!m.meta !== event.metaKey) return false;
    if (!!m.ctrl !== event.ctrlKey) return false;
    if (!!m.shift !== event.shiftKey) return false;
    if (!!m.alt !== event.altKey) return false;
    return true;
}
function isInputElement(target) {
    if (!(target instanceof Element)) return false;
    const el = target;
    const tag = el.tagName.toLowerCase();
    if (tag === "input" || tag === "textarea" || tag === "select") return true;
    if (el instanceof HTMLElement && el.isContentEditable) return true;
    return false;
}
/** Singleton registry — import this in components and hooks. */ const registry = new ShortcutRegistry();


/***/ }),

/***/ 8570:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   B5: () => (/* binding */ mapBacklogItem),
/* harmony export */   Kz: () => (/* binding */ GitHubAuthError),
/* harmony export */   Z3: () => (/* binding */ useBacklogSessionIndex),
/* harmony export */   gG: () => (/* binding */ useBacklogService)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* harmony import */ var _connectrpc_connect__WEBPACK_IMPORTED_MODULE_5__ = __webpack_require__(66144);
/* harmony import */ var _lib_api_transport__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(21694);
/* harmony import */ var _bufbuild_protobuf_wkt__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(28765);
/* harmony import */ var _lib_utils_connectError__WEBPACK_IMPORTED_MODULE_6__ = __webpack_require__(8854);
/* harmony import */ var _lib_hooks_useAbortableEffect__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(64343);
/* harmony import */ var _gen_session_v1_backlog_pb__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(79870);
/* __next_internal_client_entry_do_not_use__ mapBacklogItem,GitHubAuthError,useBacklogService,useBacklogSessionIndex auto */ 






/** Mirrors the server's maxKnownRepoPaths (server/services/backlog_service_intent.go); the server is authoritative. */ const MAX_KNOWN_REPO_PATHS = 20;
// ---------------------------------------------------------------------------
// Proto ↔ domain mapping helpers
// ---------------------------------------------------------------------------
function toItemDependency(d) {
    return {
        itemId: d.itemId,
        publicId: d.publicId,
        title: d.title,
        status: d.status
    };
}
function mapAcCriterion(c) {
    return {
        index: c.index,
        text: c.text,
        status: c.status || "pending"
    };
}
function mapItemSession(s) {
    var _s_commitCountSinceSpawn, _s_estimatedCostUsd, _s_pipelineModeSnapshot, _s_pipelineModeSnapshotHash, _s_executorSnapshotHash;
    const session = {
        entityId: s.id,
        sessionId: s.sessionUuid,
        role: s.sessionRole,
        startedAt: s.startedAt ? new Date(Number(s.startedAt.seconds) * 1000).toISOString() : undefined,
        endedAt: s.endedAt ? new Date(Number(s.endedAt.seconds) * 1000).toISOString() : undefined,
        commitCountSinceSpawn: (_s_commitCountSinceSpawn = s.commitCountSinceSpawn) !== null && _s_commitCountSinceSpawn !== void 0 ? _s_commitCountSinceSpawn : 0,
        lastCommitAt: s.lastCommitAt ? (0,_bufbuild_protobuf_wkt__WEBPACK_IMPORTED_MODULE_4__/* .timestampDate */ .FP)(s.lastCommitAt).toISOString() : undefined,
        lastCommitMessage: s.lastCommitMessage || undefined,
        lastFileTouchAt: s.lastFileTouchAt ? (0,_bufbuild_protobuf_wkt__WEBPACK_IMPORTED_MODULE_4__/* .timestampDate */ .FP)(s.lastFileTouchAt).toISOString() : undefined,
        estimatedCostUsd: (_s_estimatedCostUsd = s.estimatedCostUsd) !== null && _s_estimatedCostUsd !== void 0 ? _s_estimatedCostUsd : 0,
        worktreeBranch: s.worktreeBranch || undefined,
        worktreePath: s.worktreePath || undefined,
        baseCommitSha: s.baseCommitSha || undefined,
        pipelineModeSnapshot: (_s_pipelineModeSnapshot = s.pipelineModeSnapshot) !== null && _s_pipelineModeSnapshot !== void 0 ? _s_pipelineModeSnapshot : "",
        pipelineModeSnapshotHash: (_s_pipelineModeSnapshotHash = s.pipelineModeSnapshotHash) !== null && _s_pipelineModeSnapshotHash !== void 0 ? _s_pipelineModeSnapshotHash : "",
        endReason: s.endReason || undefined,
        failureCapturePath: s.failureCapturePath || undefined,
        errorDetail: s.errorDetail || undefined,
        resolvedProgram: s.resolvedProgram || undefined,
        resolvedModel: s.resolvedModel || undefined,
        executorSnapshotHash: (_s_executorSnapshotHash = s.executorSnapshotHash) !== null && _s_executorSnapshotHash !== void 0 ? _s_executorSnapshotHash : "",
        configuredProgram: s.configuredProgram || undefined,
        executorFallbackReason: s.executorFallbackReason || undefined
    };
    // Map review verdict if present
    if (s.reviewVerdict) {
        const rv = s.reviewVerdict;
        const knownOutcomes = new Set([
            "PASS",
            "FAIL",
            "PARTIAL",
            "UNVERIFIABLE"
        ]);
        var _rv_perCriterion;
        session.reviewVerdict = {
            overallOutcome: knownOutcomes.has(rv.overallOutcome) ? rv.overallOutcome : rv.overallOutcome ? "PARTIAL" : "PENDING",
            summary: rv.summary,
            perCriterion: ((_rv_perCriterion = rv.perCriterion) !== null && _rv_perCriterion !== void 0 ? _rv_perCriterion : []).map((c)=>({
                    criterionIndex: c.criterionIndex,
                    outcome: c.outcome,
                    evidence: c.evidence
                }))
        };
    }
    // Map triage result if present
    if (s.triageResult) {
        const tr = s.triageResult;
        var _tr_suggestions, _tr_clarifyingQuestions, _tr_tasks;
        session.triageResult = {
            summary: tr.summary,
            suggestions: ((_tr_suggestions = tr.suggestions) !== null && _tr_suggestions !== void 0 ? _tr_suggestions : []).map((sg)=>({
                    text: sg.text,
                    rationale: sg.rationale
                })),
            clarifyingQuestions: (_tr_clarifyingQuestions = tr.clarifyingQuestions) !== null && _tr_clarifyingQuestions !== void 0 ? _tr_clarifyingQuestions : [],
            tasks: ((_tr_tasks = tr.tasks) !== null && _tr_tasks !== void 0 ? _tr_tasks : []).map((t)=>({
                    text: t.text,
                    estimate: t.estimate,
                    category: t.category
                })),
            iteration: tr.iteration,
            feedback: tr.feedback
        };
    }
    return session;
}
function mapStatusEvent(e) {
    return {
        id: e.id,
        fromStatus: e.fromStatus,
        toStatus: e.toStatus,
        triggeredBy: e.triggeredBy,
        createdAt: e.createdAt ? new Date(Number(e.createdAt.seconds) * 1000).toISOString() : undefined,
        note: e.note
    };
}
function mapProgressNote(n) {
    return {
        id: n.id,
        criterionIndex: n.criterionIndex,
        note: n.note,
        status: n.status,
        createdAt: n.createdAt ? new Date(Number(n.createdAt.seconds) * 1000).toISOString() : undefined
    };
}
function mapActivityNote(n) {
    return {
        id: n.id,
        message: n.message,
        authorSessionUuid: n.authorSessionUuid,
        authorSessionTitle: n.authorSessionTitle,
        createdAt: n.createdAt ? new Date(Number(n.createdAt.seconds) * 1000).toISOString() : undefined
    };
}
/** Strips the protobuf Message<> wrapper down to the plain {program, model} shape PipelineModeForm consumes. */ function mapStageExecutors(raw) {
    const result = {};
    if (!raw) return result;
    for (const [role, executor] of Object.entries(raw)){
        result[role] = {
            program: executor.program,
            model: executor.model
        };
    }
    return result;
}
function mapPipelineMode(p) {
    return {
        id: p.id,
        slug: p.slug,
        name: p.name,
        description: p.description,
        enabled: p.enabled,
        statusCommandTemplate: p.statusCommandTemplate,
        doneCommandTemplate: p.doneCommandTemplate,
        failCommandTemplate: p.failCommandTemplate,
        reviewCommandTemplate: p.reviewCommandTemplate,
        shipCommandTemplate: p.shipCommandTemplate,
        helpCommandTemplate: p.helpCommandTemplate,
        triagePromptTemplate: p.triagePromptTemplate,
        reviewPromptTemplate: p.reviewPromptTemplate,
        initialPromptTemplate: p.initialPromptTemplate,
        contentHash: p.contentHash,
        stageExecutors: mapStageExecutors(p.stageExecutors),
        stageExecutorHashes: {
            ...p.stageExecutorHashes
        }
    };
}
// Exported so other real-time consumers (useWatchBacklogItems) can convert
// the raw proto BacklogItem their stream/store deals in to this file's mapped
// domain BacklogItem — the shape BacklogItemCard/BacklogBoard/BacklogItemDetail
// actually render (acCriteria, gateVerdict, triageStatus, ISO date strings,
// etc., none of which exist on the raw proto message). Also exported for
// direct unit testing of triageStatus derivation — see
// useBacklogService.test.ts.
function mapBacklogItem(p) {
    var _mostRecentReviewSession_reviewVerdict;
    var _p_itemSessions;
    const linkedSessions = ((_p_itemSessions = p.itemSessions) !== null && _p_itemSessions !== void 0 ? _p_itemSessions : []).map(mapItemSession);
    // Extract gate verdict from the most recent session (for review status)
    let gateVerdict;
    let gateVerdictSummary;
    let gateCriteria;
    // Use the most recent review session's verdict — not the most recent session of any
    // role, which could be a work session (no verdict) after a reopen-for-revision cycle.
    const mostRecentReviewSession = linkedSessions.filter((s)=>s.role === "review").at(-1);
    if (mostRecentReviewSession === null || mostRecentReviewSession === void 0 ? void 0 : (_mostRecentReviewSession_reviewVerdict = mostRecentReviewSession.reviewVerdict) === null || _mostRecentReviewSession_reviewVerdict === void 0 ? void 0 : _mostRecentReviewSession_reviewVerdict.overallOutcome) {
        var _mostRecentReviewSession_reviewVerdict_perCriterion;
        gateVerdict = mostRecentReviewSession.reviewVerdict.overallOutcome;
        gateVerdictSummary = mostRecentReviewSession.reviewVerdict.summary;
        if ((_mostRecentReviewSession_reviewVerdict_perCriterion = mostRecentReviewSession.reviewVerdict.perCriterion) === null || _mostRecentReviewSession_reviewVerdict_perCriterion === void 0 ? void 0 : _mostRecentReviewSession_reviewVerdict_perCriterion.length) {
            gateCriteria = mostRecentReviewSession.reviewVerdict.perCriterion.map((c)=>({
                    label: c.evidence ? "".concat(c.outcome, ": ").concat(c.evidence) : "Criterion ".concat(c.criterionIndex, ": ").concat(c.outcome),
                    passed: c.outcome === "PASS" || c.outcome === "pass"
                }));
        }
    }
    // Derive triageStatus from linked sessions.
    // P12 fix: only mark "completed" if the session ended AND has a non-empty summary.
    // Orphan detection: a triage session without endedAt is only "running" while the item
    // is in "idea" status. If the item has advanced (ready, in_progress, etc.) the session
    // died without cleanly recording its end — treat it as "failed" so the UI doesn't show
    // a loading indicator for a session that no longer exists.
    const itemStatus = p.status || "idea";
    let triageStatus;
    const triageSession = linkedSessions.filter((s)=>s.role === "triage").at(-1);
    if (triageSession) {
        if (triageSession.endedAt) {
            var _triageSession_triageResult;
            triageStatus = ((_triageSession_triageResult = triageSession.triageResult) === null || _triageSession_triageResult === void 0 ? void 0 : _triageSession_triageResult.summary) ? "completed" : "failed";
        } else if (itemStatus === "idea") {
            triageStatus = "running";
        } else {
            triageStatus = "failed";
        }
    }
    const triageResult = triageSession === null || triageSession === void 0 ? void 0 : triageSession.triageResult;
    var _p_acceptanceCriteria, _p_statusEvents, _p_progressNotes, _p_activityNotes, _p_totalEstimatedCostUsd, _p_blockedBy, _p_blocking, _p_labels, _p_allowedTransitions;
    return {
        id: p.id,
        publicId: p.publicId || undefined,
        title: p.title,
        description: p.description || undefined,
        status: p.status || "idea",
        priority: p.priority || 3,
        repoPath: p.repoPath || undefined,
        skipPlanning: p.skipPlanning,
        skipReviewGate: p.skipReviewGate,
        autoSpawnSession: p.autoSpawnSession,
        autoCreatePR: p.autoCreatePr,
        autoApprovePlan: p.autoApprovePlan,
        planApproved: p.planApproved,
        planArtifactsPath: p.planArtifactsPath || undefined,
        planRejectionReason: p.planRejectionReason || undefined,
        // timestampDate, not a hand-rolled `Number(seconds) * 1000` — see the
        // createdAt/updatedAt comment above for why.
        planRejectedAt: p.planRejectedAt ? (0,_bufbuild_protobuf_wkt__WEBPACK_IMPORTED_MODULE_4__/* .timestampDate */ .FP)(p.planRejectedAt).toISOString() : undefined,
        acCriteria: ((_p_acceptanceCriteria = p.acceptanceCriteria) !== null && _p_acceptanceCriteria !== void 0 ? _p_acceptanceCriteria : []).map(mapAcCriterion),
        linkedSessions,
        notes: p.notes || undefined,
        // timestampDate (not a hand-rolled `Number(seconds) * 1000`) — the
        // previous conversion silently dropped the sub-second `nanos` field
        // entirely, truncating to the whole second. Still only millisecond
        // precision (a JS Date's ceiling) for display purposes — updatedAtRaw
        // below is the lossless form used for exact-equality CAS checks.
        createdAt: p.createdAt ? (0,_bufbuild_protobuf_wkt__WEBPACK_IMPORTED_MODULE_4__/* .timestampDate */ .FP)(p.createdAt).toISOString() : undefined,
        updatedAt: p.updatedAt ? (0,_bufbuild_protobuf_wkt__WEBPACK_IMPORTED_MODULE_4__/* .timestampDate */ .FP)(p.updatedAt).toISOString() : undefined,
        updatedAtRaw: p.updatedAt,
        gateVerdict,
        gateVerdictSummary,
        gateCriteria,
        triageStatus,
        triageResult,
        statusEvents: ((_p_statusEvents = p.statusEvents) !== null && _p_statusEvents !== void 0 ? _p_statusEvents : []).map(mapStatusEvent),
        progressNotes: ((_p_progressNotes = p.progressNotes) !== null && _p_progressNotes !== void 0 ? _p_progressNotes : []).map(mapProgressNote),
        activityNotes: ((_p_activityNotes = p.activityNotes) !== null && _p_activityNotes !== void 0 ? _p_activityNotes : []).map(mapActivityNote),
        totalEstimatedCostUsd: (_p_totalEstimatedCostUsd = p.totalEstimatedCostUsd) !== null && _p_totalEstimatedCostUsd !== void 0 ? _p_totalEstimatedCostUsd : 0,
        prUrl: p.prUrl || undefined,
        prNumber: p.prNumber || undefined,
        pipelineMode: p.pipelineMode || undefined,
        category: p.category || undefined,
        reworkCapOverride: p.reworkCapOverride,
        costBudgetThresholdUsd: p.costBudgetThresholdUsd,
        duplicatePending: p.duplicatePending,
        duplicateRef: p.duplicateRef,
        blockedBy: ((_p_blockedBy = p.blockedBy) !== null && _p_blockedBy !== void 0 ? _p_blockedBy : []).map(toItemDependency),
        blocking: ((_p_blocking = p.blocking) !== null && _p_blocking !== void 0 ? _p_blocking : []).map(toItemDependency),
        externalId: p.externalId || undefined,
        pluginId: p.pluginId || undefined,
        hasPendingGuidanceRequests: p.hasPendingGuidanceRequests || undefined,
        externalUrl: p.externalUrl || undefined,
        labels: (_p_labels = p.labels) !== null && _p_labels !== void 0 ? _p_labels : [],
        allowedTransitions: (_p_allowedTransitions = p.allowedTransitions) !== null && _p_allowedTransitions !== void 0 ? _p_allowedTransitions : []
    };
}
function toProtoAcCriteria(criteria) {
    return criteria.map((c)=>({
            $typeName: "session.v1.AcCriterion",
            index: c.index,
            text: c.text,
            status: c.status
        }));
}
// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------
// ---------------------------------------------------------------------------
// GitHub picker domain types
// ---------------------------------------------------------------------------
function toClaimedElsewhere(claim) {
    return {
        externalUrl: claim.externalUrl,
        claimingHostId: claim.claimingHostId,
        itemDeepLink: claim.itemDeepLink,
        disputed: claim.disputed
    };
}
class GitHubAuthError extends Error {
    constructor(){
        super("No GitHub token configured");
        this.name = "GitHubAuthError";
    }
}
function useBacklogService() {
    const clientRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(null);
    const [lastError, setLastError] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(null);
    const clearError = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(()=>setLastError(null), []);
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        clientRef.current = (0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_5__/* .createClient */ .UU)(_gen_session_v1_backlog_pb__WEBPACK_IMPORTED_MODULE_3__/* .BacklogService */ .jSM, (0,_lib_api_transport__WEBPACK_IMPORTED_MODULE_1__/* .getConnectTransport */ .qW)());
    }, []);
    const listBacklogItems = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (filter)=>{
        if (!clientRef.current) return [];
        try {
            var _filter_statuses, _filter_priorities, _filter_includeTerminal, _filter_includeArchived;
            const resp = await clientRef.current.listBacklogItems({
                status: (_filter_statuses = filter === null || filter === void 0 ? void 0 : filter.statuses) !== null && _filter_statuses !== void 0 ? _filter_statuses : [],
                priority: (_filter_priorities = filter === null || filter === void 0 ? void 0 : filter.priorities) !== null && _filter_priorities !== void 0 ? _filter_priorities : [],
                includeTerminal: (_filter_includeTerminal = filter === null || filter === void 0 ? void 0 : filter.includeTerminal) !== null && _filter_includeTerminal !== void 0 ? _filter_includeTerminal : false,
                includeArchived: (_filter_includeArchived = filter === null || filter === void 0 ? void 0 : filter.includeArchived) !== null && _filter_includeArchived !== void 0 ? _filter_includeArchived : false,
                sortBy: ""
            });
            var _resp_items;
            const items = ((_resp_items = resp.items) !== null && _resp_items !== void 0 ? _resp_items : []).map(mapBacklogItem);
            if (filter === null || filter === void 0 ? void 0 : filter.search) {
                const q = filter.search.toLowerCase();
                return items.filter((item)=>{
                    var _item_description;
                    return item.title.toLowerCase().includes(q) || ((_item_description = item.description) === null || _item_description === void 0 ? void 0 : _item_description.toLowerCase().includes(q));
                });
            }
            return items;
        } catch (err) {
            console.error("[useBacklogService] listBacklogItems:", err);
            return [];
        }
    }, []);
    const getBacklogItem = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (id)=>{
        if (!clientRef.current) return null;
        try {
            const resp = await clientRef.current.getBacklogItem({
                itemId: id
            });
            return resp.item ? mapBacklogItem(resp.item) : null;
        } catch (err) {
            console.error("[useBacklogService] getBacklogItem:", err);
            return null;
        }
    }, []);
    const createBacklogItem = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (data)=>{
        if (!clientRef.current) return null;
        try {
            setLastError(null);
            var _data_description, _data_repoPath, _data_priority, _data_skipPlanning, _data_skipReviewGate, _data_autoSpawnSession, _data_autoCreatePR, _data_autoApprovePlan, _data_acCriteria, _data_notes, _data_skipTriage, _data_pipelineMode, _data_category;
            const resp = await clientRef.current.createBacklogItem({
                title: data.title,
                description: (_data_description = data.description) !== null && _data_description !== void 0 ? _data_description : "",
                repoPath: (_data_repoPath = data.repoPath) !== null && _data_repoPath !== void 0 ? _data_repoPath : "",
                priority: (_data_priority = data.priority) !== null && _data_priority !== void 0 ? _data_priority : 3,
                skipPlanning: (_data_skipPlanning = data.skipPlanning) !== null && _data_skipPlanning !== void 0 ? _data_skipPlanning : false,
                skipReviewGate: (_data_skipReviewGate = data.skipReviewGate) !== null && _data_skipReviewGate !== void 0 ? _data_skipReviewGate : false,
                autoSpawnSession: (_data_autoSpawnSession = data.autoSpawnSession) !== null && _data_autoSpawnSession !== void 0 ? _data_autoSpawnSession : false,
                autoCreatePr: (_data_autoCreatePR = data.autoCreatePR) !== null && _data_autoCreatePR !== void 0 ? _data_autoCreatePR : false,
                autoApprovePlan: (_data_autoApprovePlan = data.autoApprovePlan) !== null && _data_autoApprovePlan !== void 0 ? _data_autoApprovePlan : false,
                acceptanceCriteria: toProtoAcCriteria((_data_acCriteria = data.acCriteria) !== null && _data_acCriteria !== void 0 ? _data_acCriteria : []),
                notes: (_data_notes = data.notes) !== null && _data_notes !== void 0 ? _data_notes : "",
                skipTriage: (_data_skipTriage = data.skipTriage) !== null && _data_skipTriage !== void 0 ? _data_skipTriage : false,
                pipelineMode: (_data_pipelineMode = data.pipelineMode) !== null && _data_pipelineMode !== void 0 ? _data_pipelineMode : "",
                category: (_data_category = data.category) !== null && _data_category !== void 0 ? _data_category : ""
            });
            return resp.item ? {
                item: mapBacklogItem(resp.item),
                triageTriggered: resp.triageTriggered
            } : null;
        } catch (err) {
            console.error("[useBacklogService] createBacklogItem:", err);
            setLastError(new Error((0,_lib_utils_connectError__WEBPACK_IMPORTED_MODULE_6__/* .getErrorMessage */ .u)(err, "Failed to create backlog item.")));
            return null;
        }
    }, []);
    const createBacklogItemFromChat = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (message, existingItemId)=>{
        if (!clientRef.current) return null;
        try {
            setLastError(null);
            const resp = await clientRef.current.createBacklogItemFromChat({
                message,
                existingItemId: existingItemId !== null && existingItemId !== void 0 ? existingItemId : ""
            });
            return resp.item ? {
                item: mapBacklogItem(resp.item),
                triageTriggered: resp.triageTriggered
            } : null;
        } catch (err) {
            console.error("[useBacklogService] createBacklogItemFromChat:", err);
            setLastError(new Error((0,_lib_utils_connectError__WEBPACK_IMPORTED_MODULE_6__/* .getErrorMessage */ .u)(err, "Failed to create backlog item from chat.")));
            return null;
        }
    }, []);
    const parseBacklogItemIntent = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(// repo_path is intentionally omitted: the omnibar has no "current repo"
    // context available before parsing.
    async function(message) {
        let knownRepoPaths = arguments.length > 1 && arguments[1] !== void 0 ? arguments[1] : [];
        if (!clientRef.current) return null;
        try {
            const resp = await clientRef.current.parseBacklogItemIntent({
                message,
                knownRepoPaths: knownRepoPaths.slice(0, MAX_KNOWN_REPO_PATHS)
            });
            if (resp.error || !resp.draft) return null;
            var _resp_draft_suggestedRepoPath;
            return {
                title: resp.draft.title,
                description: resp.draft.description,
                acceptanceCriteria: resp.draft.acceptanceCriteria,
                confidence: resp.draft.confidence,
                suggestedRepoPath: (_resp_draft_suggestedRepoPath = resp.draft.suggestedRepoPath) !== null && _resp_draft_suggestedRepoPath !== void 0 ? _resp_draft_suggestedRepoPath : ""
            };
        } catch (err) {
            console.error("[useBacklogService] parseBacklogItemIntent:", err);
            return null;
        }
    }, []);
    const updateBacklogItem = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (id, data)=>{
        if (!clientRef.current) return null;
        try {
            setLastError(null);
            const resp = await clientRef.current.updateBacklogItem({
                itemId: id,
                title: data.title,
                description: data.description,
                repoPath: data.repoPath,
                priority: data.priority,
                skipPlanning: data.skipPlanning,
                skipReviewGate: data.skipReviewGate,
                autoSpawnSession: data.autoSpawnSession,
                autoCreatePr: data.autoCreatePR,
                autoApprovePlan: data.autoApprovePlan,
                acceptanceCriteria: data.acCriteria ? toProtoAcCriteria(data.acCriteria) : undefined,
                notes: data.notes,
                pipelineMode: data.pipelineMode,
                category: data.category,
                reworkCapOverride: data.reworkCapOverride,
                costBudgetThresholdUsd: data.costBudgetThresholdUsd,
                prUrl: data.prUrl,
                prNumber: data.prNumber
            });
            return resp.item ? mapBacklogItem(resp.item) : null;
        } catch (err) {
            console.error("[useBacklogService] updateBacklogItem:", err);
            setLastError(new Error((0,_lib_utils_connectError__WEBPACK_IMPORTED_MODULE_6__/* .getErrorMessage */ .u)(err, "Failed to update backlog item.")));
            return null;
        }
    }, []);
    const archiveBacklogItem = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (id)=>{
        if (!clientRef.current) return false;
        try {
            await clientRef.current.archiveBacklogItem({
                itemId: id
            });
            return true;
        } catch (err) {
            console.error("[useBacklogService] archiveBacklogItem:", err);
            setLastError(new Error((0,_lib_utils_connectError__WEBPACK_IMPORTED_MODULE_6__/* .getErrorMessage */ .u)(err, "Failed to archive backlog item.")));
            throw err;
        }
    }, []);
    const unarchiveBacklogItem = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (id)=>{
        if (!clientRef.current) return false;
        try {
            await clientRef.current.unarchiveBacklogItem({
                itemId: id
            });
            return true;
        } catch (err) {
            console.error("[useBacklogService] unarchiveBacklogItem:", err);
            setLastError(new Error((0,_lib_utils_connectError__WEBPACK_IMPORTED_MODULE_6__/* .getErrorMessage */ .u)(err, "Failed to unarchive backlog item.")));
            throw err;
        }
    }, []);
    const deleteBacklogItem = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (id)=>{
        if (!clientRef.current) return false;
        try {
            await clientRef.current.deleteBacklogItem({
                itemId: id
            });
            return true;
        } catch (err) {
            console.error("[useBacklogService] deleteBacklogItem:", err);
            setLastError(new Error((0,_lib_utils_connectError__WEBPACK_IMPORTED_MODULE_6__/* .getErrorMessage */ .u)(err, "Failed to delete backlog item.")));
            throw err;
        }
    }, []);
    const transitionStatus = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (id, toStatus, options)=>{
        if (!clientRef.current) return null;
        try {
            setLastError(null);
            var _options_expectedStatus, _options_overrideReason;
            const resp = await clientRef.current.transitionBacklogItemStatus({
                itemId: id,
                targetStatus: toStatus,
                expectedStatus: (_options_expectedStatus = options === null || options === void 0 ? void 0 : options.expectedStatus) !== null && _options_expectedStatus !== void 0 ? _options_expectedStatus : "",
                // Passed through verbatim — no Date round-trip. See the
                // expectedUpdatedAt doc comment above for why that matters.
                expectedUpdatedAt: options === null || options === void 0 ? void 0 : options.expectedUpdatedAt,
                overrideReason: (_options_overrideReason = options === null || options === void 0 ? void 0 : options.overrideReason) !== null && _options_overrideReason !== void 0 ? _options_overrideReason : ""
            });
            return resp.item ? mapBacklogItem(resp.item) : null;
        } catch (err) {
            console.error("[useBacklogService] transitionStatus:", err);
            setLastError(new Error((0,_lib_utils_connectError__WEBPACK_IMPORTED_MODULE_6__/* .getErrorMessage */ .u)(err, "Failed to transition backlog item status.")));
            throw err;
        }
    }, []);
    const spawnSessionFromItem = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (id, options)=>{
        if (!clientRef.current) return null;
        try {
            setLastError(null);
            var _options_autonomous, _options_force;
            const resp = await clientRef.current.spawnSessionFromItem({
                itemId: id,
                autonomous: (_options_autonomous = options === null || options === void 0 ? void 0 : options.autonomous) !== null && _options_autonomous !== void 0 ? _options_autonomous : false,
                force: (_options_force = options === null || options === void 0 ? void 0 : options.force) !== null && _options_force !== void 0 ? _options_force : false
            });
            return {
                sessionUuid: resp.sessionUuid,
                queued: resp.queued
            };
        } catch (err) {
            console.error("[useBacklogService] spawnSessionFromItem:", err);
            setLastError(new Error((0,_lib_utils_connectError__WEBPACK_IMPORTED_MODULE_6__/* .getErrorMessage */ .u)(err, "Failed to spawn session from item.")));
            throw err;
        }
    }, []);
    const triggerTriage = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (id, feedback)=>{
        if (!clientRef.current) return null;
        try {
            var _resp_itemSession;
            const resp = await clientRef.current.triggerTriage({
                itemId: id,
                feedback: feedback !== null && feedback !== void 0 ? feedback : ""
            });
            var _resp_itemSession_id;
            return {
                itemSessionId: (_resp_itemSession_id = (_resp_itemSession = resp.itemSession) === null || _resp_itemSession === void 0 ? void 0 : _resp_itemSession.id) !== null && _resp_itemSession_id !== void 0 ? _resp_itemSession_id : ""
            };
        } catch (err) {
            console.error("[useBacklogService] triggerTriage:", err);
            setLastError(new Error((0,_lib_utils_connectError__WEBPACK_IMPORTED_MODULE_6__/* .getErrorMessage */ .u)(err, "Failed to trigger triage.")));
            throw err;
        }
    }, []);
    const cancelTriage = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (id)=>{
        if (!clientRef.current) return false;
        try {
            const resp = await clientRef.current.cancelTriage({
                itemId: id
            });
            return resp.cancelled;
        } catch (err) {
            console.error("[useBacklogService] cancelTriage:", err);
            setLastError(new Error((0,_lib_utils_connectError__WEBPACK_IMPORTED_MODULE_6__/* .getErrorMessage */ .u)(err, "Failed to cancel triage.")));
            throw err;
        }
    }, []);
    const approvePlan = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (id)=>{
        if (!clientRef.current) return null;
        try {
            const resp = await clientRef.current.approvePlan({
                itemId: id
            });
            return resp.item ? mapBacklogItem(resp.item) : null;
        } catch (err) {
            console.error("[useBacklogService] approvePlan:", err);
            setLastError(new Error((0,_lib_utils_connectError__WEBPACK_IMPORTED_MODULE_6__/* .getErrorMessage */ .u)(err, "Failed to approve plan.")));
            throw err;
        }
    }, []);
    const rejectPlan = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (id, reason)=>{
        if (!clientRef.current) return null;
        try {
            const resp = await clientRef.current.rejectPlan({
                itemId: id,
                reason
            });
            return resp.item ? mapBacklogItem(resp.item) : null;
        } catch (err) {
            console.error("[useBacklogService] rejectPlan:", err);
            setLastError(new Error((0,_lib_utils_connectError__WEBPACK_IMPORTED_MODULE_6__/* .getErrorMessage */ .u)(err, "Failed to reject plan.")));
            throw err;
        }
    }, []);
    const overrideVerdict = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (id, overrideReason, toStatus)=>{
        if (!clientRef.current) return false;
        try {
            await clientRef.current.overrideVerdict({
                itemSessionId: id,
                overrideReason,
                toStatus: toStatus !== null && toStatus !== void 0 ? toStatus : "done"
            });
            return true;
        } catch (err) {
            console.error("[useBacklogService] overrideVerdict:", err);
            setLastError(new Error((0,_lib_utils_connectError__WEBPACK_IMPORTED_MODULE_6__/* .getErrorMessage */ .u)(err, "Failed to override verdict.")));
            throw err;
        }
    }, []);
    const triggerReReview = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (id)=>{
        if (!clientRef.current) return false;
        try {
            await clientRef.current.triggerReReview({
                itemId: id
            });
            return true;
        } catch (err) {
            console.error("[useBacklogService] triggerReReview:", err);
            setLastError(new Error((0,_lib_utils_connectError__WEBPACK_IMPORTED_MODULE_6__/* .getErrorMessage */ .u)(err, "Failed to trigger re-review.")));
            throw err;
        }
    }, []);
    /**
   * Runs the same one-shot PR-creation prompt the opt-in AutoCreatePR policy
   * uses automatically, for an item in review with no PR yet — the
   * self-service "Ship PR" action on the item detail page. Can take a while
   * (the underlying RunOneShot call may run for several minutes), matching
   * ReviewQueuePanel's existing manual "Create PR" flow. Rethrows on failure
   * so the caller can show the specific error (e.g. "work session not
   * running") rather than a generic failure state.
   */ const triggerShipPR = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (id)=>{
        if (!clientRef.current) return null;
        try {
            const resp = await clientRef.current.triggerShipPR({
                itemId: id
            });
            return {
                prUrl: resp.prUrl
            };
        } catch (err) {
            console.error("[useBacklogService] triggerShipPR:", err);
            setLastError(new Error((0,_lib_utils_connectError__WEBPACK_IMPORTED_MODULE_6__/* .getErrorMessage */ .u)(err, "Failed to ship PR.")));
            throw err;
        }
    }, []);
    const submitManualReview = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (id, overallOutcome, summary)=>{
        if (!clientRef.current) return null;
        try {
            setLastError(null);
            const resp = await clientRef.current.submitManualReview({
                itemId: id,
                overallOutcome,
                summary
            });
            return resp.item ? mapBacklogItem(resp.item) : null;
        } catch (err) {
            console.error("[useBacklogService] submitManualReview:", err);
            setLastError(new Error((0,_lib_utils_connectError__WEBPACK_IMPORTED_MODULE_6__/* .getErrorMessage */ .u)(err, "Failed to submit manual review.")));
            throw err;
        }
    }, []);
    const listPipelineModes = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async ()=>{
        if (!clientRef.current) return [];
        try {
            const resp = await clientRef.current.listPipelineModes({});
            var _resp_items;
            return ((_resp_items = resp.items) !== null && _resp_items !== void 0 ? _resp_items : []).map(mapPipelineMode);
        } catch (err) {
            console.error("[useBacklogService] listPipelineModes:", err);
            throw err;
        }
    }, []);
    const getPipelineMode = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (slug)=>{
        if (!clientRef.current) return null;
        try {
            const resp = await clientRef.current.getPipelineMode({
                slug
            });
            return resp.item ? mapPipelineMode(resp.item) : null;
        } catch (err) {
            console.error("[useBacklogService] getPipelineMode:", err);
            throw err;
        }
    }, []);
    const createPipelineMode = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (data)=>{
        if (!clientRef.current) throw new Error("Backlog service not connected");
        try {
            var _data_description, _data_enabled, _data_statusCommandTemplate, _data_doneCommandTemplate, _data_failCommandTemplate, _data_reviewCommandTemplate, _data_shipCommandTemplate, _data_helpCommandTemplate, _data_triagePromptTemplate, _data_reviewPromptTemplate, _data_initialPromptTemplate, _data_stageExecutors, _data_forceUnknownModel;
            const resp = await clientRef.current.createPipelineMode({
                slug: data.slug,
                name: data.name,
                description: (_data_description = data.description) !== null && _data_description !== void 0 ? _data_description : "",
                enabled: (_data_enabled = data.enabled) !== null && _data_enabled !== void 0 ? _data_enabled : false,
                statusCommandTemplate: (_data_statusCommandTemplate = data.statusCommandTemplate) !== null && _data_statusCommandTemplate !== void 0 ? _data_statusCommandTemplate : "",
                doneCommandTemplate: (_data_doneCommandTemplate = data.doneCommandTemplate) !== null && _data_doneCommandTemplate !== void 0 ? _data_doneCommandTemplate : "",
                failCommandTemplate: (_data_failCommandTemplate = data.failCommandTemplate) !== null && _data_failCommandTemplate !== void 0 ? _data_failCommandTemplate : "",
                reviewCommandTemplate: (_data_reviewCommandTemplate = data.reviewCommandTemplate) !== null && _data_reviewCommandTemplate !== void 0 ? _data_reviewCommandTemplate : "",
                shipCommandTemplate: (_data_shipCommandTemplate = data.shipCommandTemplate) !== null && _data_shipCommandTemplate !== void 0 ? _data_shipCommandTemplate : "",
                helpCommandTemplate: (_data_helpCommandTemplate = data.helpCommandTemplate) !== null && _data_helpCommandTemplate !== void 0 ? _data_helpCommandTemplate : "",
                triagePromptTemplate: (_data_triagePromptTemplate = data.triagePromptTemplate) !== null && _data_triagePromptTemplate !== void 0 ? _data_triagePromptTemplate : "",
                reviewPromptTemplate: (_data_reviewPromptTemplate = data.reviewPromptTemplate) !== null && _data_reviewPromptTemplate !== void 0 ? _data_reviewPromptTemplate : "",
                initialPromptTemplate: (_data_initialPromptTemplate = data.initialPromptTemplate) !== null && _data_initialPromptTemplate !== void 0 ? _data_initialPromptTemplate : "",
                stageExecutors: (_data_stageExecutors = data.stageExecutors) !== null && _data_stageExecutors !== void 0 ? _data_stageExecutors : {},
                forceUnknownModel: (_data_forceUnknownModel = data.forceUnknownModel) !== null && _data_forceUnknownModel !== void 0 ? _data_forceUnknownModel : false
            });
            if (!resp.item) throw new Error("createPipelineMode: server returned no item");
            return mapPipelineMode(resp.item);
        } catch (err) {
            console.error("[useBacklogService] createPipelineMode:", err);
            throw err;
        }
    }, []);
    const updatePipelineMode = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (id, data)=>{
        if (!clientRef.current) throw new Error("Backlog service not connected");
        try {
            const resp = await clientRef.current.updatePipelineMode({
                id,
                name: data.name,
                description: data.description,
                enabled: data.enabled,
                statusCommandTemplate: data.statusCommandTemplate,
                doneCommandTemplate: data.doneCommandTemplate,
                failCommandTemplate: data.failCommandTemplate,
                reviewCommandTemplate: data.reviewCommandTemplate,
                shipCommandTemplate: data.shipCommandTemplate,
                helpCommandTemplate: data.helpCommandTemplate,
                triagePromptTemplate: data.triagePromptTemplate,
                reviewPromptTemplate: data.reviewPromptTemplate,
                initialPromptTemplate: data.initialPromptTemplate,
                // undefined = leave stage executors untouched; present (even {}) = replace.
                stageExecutors: data.stageExecutors !== undefined ? {
                    values: data.stageExecutors
                } : undefined,
                forceUnknownModel: data.forceUnknownModel
            });
            if (!resp.item) throw new Error("updatePipelineMode: server returned no item");
            return mapPipelineMode(resp.item);
        } catch (err) {
            console.error("[useBacklogService] updatePipelineMode:", err);
            throw err;
        }
    }, []);
    const deletePipelineMode = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (id)=>{
        if (!clientRef.current) return false;
        try {
            await clientRef.current.deletePipelineMode({
                id
            });
            return true;
        } catch (err) {
            console.error("[useBacklogService] deletePipelineMode:", err);
            throw err;
        }
    }, []);
    const checkCrossHostClaim = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (externalUrl, options)=>{
        if (!clientRef.current) return null;
        try {
            var _options_localOnly;
            const resp = await clientRef.current.checkCrossHostClaim({
                externalUrl,
                localOnly: (_options_localOnly = options === null || options === void 0 ? void 0 : options.localOnly) !== null && _options_localOnly !== void 0 ? _options_localOnly : false
            });
            return {
                enabled: resp.enabled,
                checked: resp.checked,
                claim: resp.claim ? toClaimedElsewhere(resp.claim) : undefined,
                claimedAtUnix: Number(resp.claimedAtUnix)
            };
        } catch (err) {
            console.warn("[useBacklogService] checkCrossHostClaim:", err);
            return null;
        }
    }, []);
    const listForeignClaims = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async ()=>{
        if (!clientRef.current) return [];
        try {
            const resp = await clientRef.current.listForeignClaims({});
            return resp.enabled ? resp.claims.map(toClaimedElsewhere) : [];
        } catch (err) {
            console.warn("[useBacklogService] listForeignClaims:", err);
            return [];
        }
    }, []);
    const resolveClaimDispute = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (externalUrl, reason)=>{
        if (!clientRef.current) throw new Error("Backlog service unavailable.");
        try {
            await clientRef.current.resolveClaimDispute({
                externalUrl,
                reason
            });
        } catch (err) {
            throw new Error((0,_lib_utils_connectError__WEBPACK_IMPORTED_MODULE_6__/* .getErrorMessage */ .u)(err, "Failed to resolve claim dispute."));
        }
    }, []);
    const importGitHubIssue = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (issueUrl, options)=>{
        if (!clientRef.current) return null;
        try {
            setLastError(null);
            var _options_repoPath, _options_skipPlanning, _options_overrideReason;
            const resp = await clientRef.current.importGitHubIssue({
                issueUrl,
                repoPath: (_options_repoPath = options === null || options === void 0 ? void 0 : options.repoPath) !== null && _options_repoPath !== void 0 ? _options_repoPath : "",
                skipPlanning: (_options_skipPlanning = options === null || options === void 0 ? void 0 : options.skipPlanning) !== null && _options_skipPlanning !== void 0 ? _options_skipPlanning : false,
                override: (options === null || options === void 0 ? void 0 : options.overrideReason) !== undefined,
                overrideReason: (_options_overrideReason = options === null || options === void 0 ? void 0 : options.overrideReason) !== null && _options_overrideReason !== void 0 ? _options_overrideReason : ""
            });
            if (resp.alreadyClaimedElsewhere) {
                const claim = resp.alreadyClaimedElsewhere;
                return {
                    alreadyClaimedElsewhere: {
                        externalUrl: claim.externalUrl,
                        claimingHostId: claim.claimingHostId,
                        itemDeepLink: claim.itemDeepLink,
                        disputed: claim.disputed
                    }
                };
            }
            return resp.item ? {
                item: mapBacklogItem(resp.item),
                triageTriggered: resp.triageTriggered,
                alreadyExisted: resp.alreadyExisted
            } : null;
        } catch (err) {
            console.error("[useBacklogService] importGitHubIssue:", err);
            setLastError(new Error((0,_lib_utils_connectError__WEBPACK_IMPORTED_MODULE_6__/* .getErrorMessage */ .u)(err, "Failed to import GitHub issue.")));
            return null;
        }
    }, []);
    const searchGitHubRepos = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (query, limit)=>{
        if (!clientRef.current) return [];
        try {
            const resp = await clientRef.current.searchGitHubRepos({
                query,
                limit: limit !== null && limit !== void 0 ? limit : 30
            });
            return resp.repos.map((r)=>({
                    owner: r.owner,
                    repo: r.repo,
                    isLocal: r.isLocal,
                    localPath: r.localPath,
                    description: r.description,
                    host: r.host
                }));
        } catch (err) {
            if (err instanceof Error && err.message.toLowerCase().includes("token")) {
                throw new GitHubAuthError();
            }
            throw err;
        }
    }, []);
    const listGitHubIssues = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (owner, repo, options)=>{
        if (!clientRef.current) return [];
        try {
            var _options_state, _options_search, _options_limit, _options_host;
            const resp = await clientRef.current.listGitHubIssues({
                owner,
                repo,
                state: (_options_state = options === null || options === void 0 ? void 0 : options.state) !== null && _options_state !== void 0 ? _options_state : "open",
                search: (_options_search = options === null || options === void 0 ? void 0 : options.search) !== null && _options_search !== void 0 ? _options_search : "",
                limit: (_options_limit = options === null || options === void 0 ? void 0 : options.limit) !== null && _options_limit !== void 0 ? _options_limit : 30,
                host: (_options_host = options === null || options === void 0 ? void 0 : options.host) !== null && _options_host !== void 0 ? _options_host : ""
            });
            return resp.issues.map((i)=>{
                var _i_isPr;
                return {
                    number: i.number,
                    title: i.title,
                    body: i.body,
                    author: i.author,
                    state: i.state,
                    url: i.url,
                    labels: i.labels,
                    createdAt: i.createdAt ? new Date(Number(i.createdAt.seconds) * 1000).toISOString() : undefined,
                    updatedAt: i.updatedAt ? new Date(Number(i.updatedAt.seconds) * 1000).toISOString() : undefined,
                    isPR: (_i_isPr = i.isPr) !== null && _i_isPr !== void 0 ? _i_isPr : false,
                    host: i.host
                };
            });
        } catch (err) {
            if (err instanceof Error && err.message.toLowerCase().includes("token")) {
                throw new GitHubAuthError();
            }
            throw err;
        }
    }, []);
    // Stable object reference: all methods are useCallback(fn,[]) — only lastError changes.
    // Without useMemo, every render creates a new object, making callers' useCallback deps
    // fire on every render and causing infinite reload loops.
    return (0,react__WEBPACK_IMPORTED_MODULE_0__.useMemo)(()=>({
            listBacklogItems,
            getBacklogItem,
            createBacklogItem,
            createBacklogItemFromChat,
            parseBacklogItemIntent,
            importGitHubIssue,
            checkCrossHostClaim,
            listForeignClaims,
            resolveClaimDispute,
            searchGitHubRepos,
            listGitHubIssues,
            updateBacklogItem,
            archiveBacklogItem,
            unarchiveBacklogItem,
            deleteBacklogItem,
            transitionStatus,
            spawnSessionFromItem,
            triggerTriage,
            cancelTriage,
            approvePlan,
            rejectPlan,
            overrideVerdict,
            triggerReReview,
            triggerShipPR,
            submitManualReview,
            listPipelineModes,
            getPipelineMode,
            createPipelineMode,
            updatePipelineMode,
            deletePipelineMode,
            lastError,
            clearError
        }), // eslint-disable-next-line react-hooks/exhaustive-deps
    [
        lastError
    ]);
}
/**
 * Fetches the full session→backlog index once on mount.
 * Returns a stable Map keyed by tmux session UUID.
 */ function useBacklogSessionIndex() {
    const [index, setIndex] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(new Map());
    const [loading, setLoading] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(true);
    (0,_lib_hooks_useAbortableEffect__WEBPACK_IMPORTED_MODULE_2__/* .useAbortableEffect */ .B)(async (signal)=>{
        const client = (0,_connectrpc_connect__WEBPACK_IMPORTED_MODULE_5__/* .createClient */ .UU)(_gen_session_v1_backlog_pb__WEBPACK_IMPORTED_MODULE_3__/* .BacklogService */ .jSM, (0,_lib_api_transport__WEBPACK_IMPORTED_MODULE_1__/* .getConnectTransport */ .qW)());
        try {
            const resp = await client.getSessionBacklogIndex({}, {
                signal
            });
            if (signal.aborted) return;
            // Entries are ordered newest-first (see GetAllItemSessionsWithBacklogInfo's doc
            // comment in ent_repository_backlog.go) — keep only the first entry seen per
            // session UUID so a re-parented session (e.g. triage -> work) resolves to its
            // current role, not a stale one from an earlier row.
            const map = new Map();
            var _resp_entries;
            for (const e of (_resp_entries = resp.entries) !== null && _resp_entries !== void 0 ? _resp_entries : []){
                if (e.sessionUuid && !map.has(e.sessionUuid)) {
                    map.set(e.sessionUuid, {
                        itemId: e.itemId,
                        itemTitle: e.itemTitle,
                        itemStatus: e.itemStatus,
                        sessionRole: e.sessionRole
                    });
                }
            }
            setIndex(map);
        } catch (err) {
            if (signal.aborted) return;
            console.error("[useBacklogSessionIndex] failed:", err);
        } finally{
            if (!signal.aborted) setLoading(false);
        }
    }, []);
    return {
        index,
        loading
    };
}


/***/ }),

/***/ 13468:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   I8: () => (/* binding */ usePushClickHandoff),
/* harmony export */   c$: () => (/* binding */ usePushNotifications)
/* harmony export */ });
/* unused harmony exports NOTIFICATION_CLICK_MESSAGE, notificationClickTarget */
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* __next_internal_client_entry_do_not_use__ NOTIFICATION_CLICK_MESSAGE,notificationClickTarget,usePushClickHandoff,usePushNotifications auto */ 
/** Message the push service worker posts to an open window on `notificationclick` (Story 5.5). */ const NOTIFICATION_CLICK_MESSAGE = "notification-click";
/** In-app target of a service-worker click message, or null when it is not one or is not a same-origin path. */ function notificationClickTarget(data) {
    const msg = data;
    if (!msg || msg.type !== NOTIFICATION_CLICK_MESSAGE || typeof msg.url !== "string") return null;
    const url = msg.url;
    return url.startsWith("/") && !url.startsWith("//") ? url : null;
}
/** Routes a push click handed off by the service worker through the router, so the page is not reloaded. */ function usePushClickHandoff(navigate) {
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        if (typeof navigator === "undefined" || !("serviceWorker" in navigator)) return;
        const sw = navigator.serviceWorker;
        const onMessage = (event)=>{
            const target = notificationClickTarget(event.data);
            if (target) navigate(target);
        };
        sw.addEventListener("message", onMessage);
        return ()=>sw.removeEventListener("message", onMessage);
    }, [
        navigate
    ]);
}
function usePushNotifications() {
    let { onNotification } = arguments.length > 0 && arguments[0] !== void 0 ? arguments[0] : {};
    const [isSupported, setIsSupported] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(false);
    const [subscription, setSubscription] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(null);
    const [isLoading, setIsLoading] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(true);
    const [error, setError] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(null);
    const [permission, setPermission] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)("default");
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        const supported = "Notification" in window && "serviceWorker" in navigator && "PushManager" in window;
        setIsSupported(supported);
        if (supported) {
            setPermission(Notification.permission);
        }
        setIsLoading(false);
    }, []);
    // Watch for permission changes (e.g., user revokes in browser settings while app is open).
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        if (!isSupported || "object" === "undefined") return;
        if (!("permissions" in navigator)) return;
        let status = null;
        navigator.permissions.query({
            name: "notifications"
        }).then((s)=>{
            status = s;
            const handler = ()=>setPermission(Notification.permission);
            s.addEventListener("change", handler);
        });
        return ()=>{
            if (status) {
                const handler = ()=>setPermission(Notification.permission);
                status.removeEventListener("change", handler);
            }
        };
    }, [
        isSupported
    ]);
    const requestPermission = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async ()=>{
        if (!isSupported) {
            setError("Push notifications not supported");
            return "denied";
        }
        try {
            const permission = await Notification.requestPermission();
            return permission;
        } catch (err) {
            setError("Failed to request notification permission");
            return "denied";
        }
    }, [
        isSupported
    ]);
    const subscribe = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (baseUrl)=>{
        if (!isSupported) {
            setError("Push notifications not supported");
            return null;
        }
        try {
            const registration = await navigator.serviceWorker.register("/push-sw.js");
            await navigator.serviceWorker.ready;
            const existingSubscription = await registration.pushManager.getSubscription();
            if (existingSubscription) {
                setSubscription(existingSubscription);
                return existingSubscription;
            }
            const vapidPublicKey = await fetch("".concat(baseUrl, "/push/vapid-key")).then((r)=>r.text());
            const newSubscription = await registration.pushManager.subscribe({
                userVisibleOnly: true,
                // @ts-ignore - PushManager.subscribe accepts Uint8Array but types are strict
                applicationServerKey: urlBase64ToUint8Array(vapidPublicKey)
            });
            await fetch("".concat(baseUrl, "/push/subscribe"), {
                method: "POST",
                headers: {
                    "Content-Type": "application/json"
                },
                body: JSON.stringify(newSubscription.toJSON())
            });
            setSubscription(newSubscription);
            return newSubscription;
        } catch (err) {
            const message = err instanceof Error ? err.message : "Failed to subscribe";
            setError(message);
            return null;
        }
    }, [
        isSupported
    ]);
    const unsubscribe = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (baseUrl)=>{
        if (!subscription) return true;
        try {
            await subscription.unsubscribe();
            await fetch("".concat(baseUrl, "/push/unsubscribe"), {
                method: "POST",
                headers: {
                    "Content-Type": "application/json"
                },
                body: JSON.stringify({
                    endpoint: subscription.endpoint
                })
            });
            setSubscription(null);
            return true;
        } catch (err) {
            setError("Failed to unsubscribe");
            return false;
        }
    }, [
        subscription
    ]);
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        if (!isSupported || "object" === "undefined") return;
        const setupServiceWorker = async ()=>{
            try {
                const registration = await navigator.serviceWorker.register("/push-sw.js");
                registration.addEventListener("updatefound", ()=>{
                    const newWorker = registration.installing;
                    if (newWorker) {
                        newWorker.addEventListener("statechange", ()=>{
                            if (newWorker.state === "installed" && navigator.serviceWorker.controller) {
                                newWorker.postMessage({
                                    type: "SKIP_WAITING"
                                });
                            }
                        });
                    }
                });
                const existingSubscription = await registration.pushManager.getSubscription();
                if (existingSubscription) {
                    setSubscription(existingSubscription);
                }
            } catch (err) {
                console.error("Service worker registration failed:", err);
            }
        };
        setupServiceWorker();
    }, [
        isSupported
    ]);
    return {
        isSupported,
        subscription,
        isLoading,
        error,
        permission,
        requestPermission,
        subscribe,
        unsubscribe
    };
}
function urlBase64ToUint8Array(base64String) {
    const padding = "=".repeat((4 - base64String.length % 4) % 4);
    const base64 = (base64String + padding).replace(/-/g, "+").replace(/_/g, "/");
    const rawData = window.atob(base64);
    const outputArray = new Uint8Array(rawData.length);
    for(let i = 0; i < rawData.length; ++i){
        outputArray[i] = rawData.charCodeAt(i);
    }
    return outputArray;
}


/***/ }),

/***/ 71048:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  u: () => (/* binding */ Kbd)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fui%2FKbd.css.ts.vanilla.css%22%2C%22source%22%3A%22Ll82dmR0Z2kwIHsKICBkaXNwbGF5OiBpbmxpbmUtZmxleDsKICBhbGlnbi1pdGVtczogY2VudGVyOwogIGp1c3RpZnktY29udGVudDogY2VudGVyOwogIGZvbnQtZmFtaWx5OiB2YXIoLS1fMTllejJyejJ1KTsKICBsaW5lLWhlaWdodDogMTsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyem0pOwogIGNvbG9yOiB2YXIoLS1fMTllejJyenEpOwogIGJvcmRlcjogMXB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6aCk7CiAgYm9yZGVyLXJhZGl1czogdmFyKC0tXzE5ZXoycnozNik7CiAgdXNlci1zZWxlY3Q6IG5vbmU7Cn0KLl82dmR0Z2kxIHsKICBmb250LXNpemU6IHZhcigtLV8xOWV6MnJ6M2EpOwogIHBhZGRpbmc6IDJweCA1cHg7CiAgbWluLXdpZHRoOiAxLjI1cmVtOwp9Ci5fNnZkdGdpMiB7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNiKTsKICBwYWRkaW5nOiAzcHggN3B4OwogIG1pbi13aWR0aDogMS41cmVtOwp9%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fui_2FKbd_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll82dmR0Z2kwIHsKICBkaXNwbGF5OiBpbmxpbmUtZmxleDsKICBhbGlnbi1pdGVtczogY2VudGVyOwogIGp1c3RpZnktY29udGVudDogY2VudGVyOwogIGZvbnQtZmFtaWx5OiB2YXIoLS1fMTllejJyejJ1KTsKICBsaW5lLWhlaWdodDogMTsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyem0pOwogIGNvbG9yOiB2YXIoLS1fMTllejJyenEpOwogIGJvcmRlcjogMXB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6aCk7CiAgYm9yZGVyLXJhZGl1czogdmFyKC0tXzE5ZXoycnozNik7CiAgdXNlci1zZWxlY3Q6IG5vbmU7Cn0KLl82dmR0Z2kxIHsKICBmb250LXNpemU6IHZhcigtLV8xOWV6MnJ6M2EpOwogIHBhZGRpbmc6IDJweCA1cHg7CiAgbWluLXdpZHRoOiAxLjI1cmVtOwp9Ci5fNnZkdGdpMiB7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNiKTsKICBwYWRkaW5nOiAzcHggN3B4OwogIG1pbi13aWR0aDogMS41cmVtOwp9_22_7D = __webpack_require__(46573);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+recipes@0.5.7+6974e6bd53375d7a/node_modules/@vanilla-extract/recipes/dist/createRuntimeFn-62c9670f.esm.js
var createRuntimeFn_62c9670f_esm = __webpack_require__(8462);
;// ./src/components/ui/Kbd.css.ts



var kbd = (0,createRuntimeFn_62c9670f_esm.c)({defaultClassName:'_6vdtgi0',variantClassNames:{size:{sm:'_6vdtgi1',md:'_6vdtgi2'}},defaultVariants:{size:'md'},compoundVariants:[]});
;// ./src/components/ui/Kbd.tsx


/**
 * Styled keyboard key component.
 * Uses theme tokens (vars.color.primary, vars.font.mono) so it adapts
 * automatically to any active vanilla-extract theme.
 */ function Kbd(param) {
    let { children, size = "md" } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsx)("kbd", {
        className: kbd({
            size
        }),
        children: children
    });
}


/***/ }),

/***/ 76514:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  J: () => (/* reexport */ routes)
});

;// ./src/lib/nav/paths.ts
/**
 * Centralized route definitions for type-safe navigation (re-exported from `@/lib/routes`)
 */ const routes = {
    home: "/",
    sessionCreate: "/?new=true",
    reviewQueue: "/review-queue",
    unfinished: "/unfinished",
    rules: "/rules",
    history: "/history",
    logs: "/logs",
    errors: "/errors",
    config: "/config",
    notifications: "/notifications",
    settings: "/settings",
    settingsDefaults: "/settings/defaults",
    help: "/help",
    changelog: "/changelog",
    settingsUnfinished: "/settings/unfinished",
    insights: "/insights",
    settingsFeatures: "/settings/features",
    settingsBacklogSources: "/settings/backlog-sources",
    settingsBacklogStages: "/settings/backlog-stages",
    settingsPipelineModes: "/settings/pipeline-modes",
    settingsRemotes: "/settings/remotes",
    settingsJules: "/settings/jules",
    settingsTaggingClassifier: "/settings/tagging-classifier",
    settingsLlmBackends: "/settings/llm-backends",
    settingsBackgroundModels: "/settings/background-models",
    backlog: "/backlog",
    backlogBoard: "/backlog/board",
    sessionsImport: "/sessions/import",
    workflows: "/workflows",
    triggers: "/triggers",
    login: "/login",
    account: "/account",
    escapeAnalytics: "/analytics/escape",
    files: "/files",
    sessionDetail: (id)=>"/?session=".concat(id),
    newSessionFromWorktree: (worktreePath, branch, title)=>{
        const params = new URLSearchParams({
            worktree: worktreePath,
            branch
        });
        if (title) params.set("title", title);
        return "/?".concat(params.toString());
    },
    unfinishedItem: (itemId)=>"/unfinished?item=".concat(encodeURIComponent(itemId)),
    unfinishedTab: (tab)=>"/unfinished?tab=".concat(encodeURIComponent(tab))
};
/** Every `page.tsx` under src/app, one literal each; `route-meta.test.ts` fails when this list drifts from the directory. */ const ROUTE_PATHS = (/* unused pure expression or super */ null && ([
    "/",
    "/account",
    "/analytics/escape",
    "/backlog",
    "/backlog/board",
    "/changelog",
    "/config",
    "/debug/escape-codes",
    "/errors",
    "/fanout",
    "/files",
    "/help",
    "/history",
    "/insights",
    "/insights/session-detail",
    "/login",
    "/logs",
    "/notifications",
    "/resolve",
    "/review-queue",
    "/rules",
    "/sessions/import",
    "/sessions/new",
    "/sessions/summary",
    "/settings",
    "/settings/background-models",
    "/settings/backlog-sources",
    "/settings/backlog-stages",
    "/settings/defaults",
    "/settings/features",
    "/settings/jules",
    "/settings/llm-backends",
    "/settings/pipeline-modes",
    "/settings/remotes",
    "/settings/tagging-classifier",
    "/settings/unfinished",
    "/spike-files",
    "/spike-lazydoc",
    "/test/escape-codes",
    "/test/layout-overlap",
    "/test/terminal-stress",
    "/triggers",
    "/unfinished",
    "/workflows"
]));

;// ./src/lib/routes.ts
/**
 * Centralized route definitions for type-safe navigation.
 * The data lives in `@/lib/nav/paths`; this path stays for existing importers.
 */ 


/***/ }),

/***/ 78475:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   FT: () => (/* binding */ chordFromEvent),
/* harmony export */   KU: () => (/* binding */ setLeaderConfig),
/* harmony export */   RN: () => (/* binding */ formatLeader),
/* harmony export */   Wh: () => (/* binding */ chordMatches),
/* harmony export */   ey: () => (/* binding */ getLeaderConfig),
/* harmony export */   o5: () => (/* binding */ LEADER_CONFIG_EVENT),
/* harmony export */   q8: () => (/* binding */ getLeaderIdleMs),
/* harmony export */   uN: () => (/* binding */ isSafeChord),
/* harmony export */   x7: () => (/* binding */ resetLeaderConfig)
/* harmony export */ });
/* unused harmony exports LEADER_STORAGE_KEY, DEFAULT_LEADER_CHORD, LEADER_IDLE_KEY, DEFAULT_LEADER_IDLE_MS, formatChord */
/**
 * Leader chord configuration for the command bar.
 *
 * Matched on KeyboardEvent.code, never .key: on macOS Option+letter produces a
 * symbol (a dead key), so .key is unusable for Alt chords. See
 * docs/reference/leader-key-chord-probe.md for how the default was chosen and
 * what has / has not been verified.
 */ const LEADER_STORAGE_KEY = "ssq.leaderKey";
const LEADER_CONFIG_EVENT = "ssq:leader-config-changed";
const DEFAULT_LEADER_CHORD = {
    code: "Semicolon",
    alt: true
};
const DEFAULT_CONFIG = {
    enabled: true,
    chord: DEFAULT_LEADER_CHORD
};
const MODIFIER_CODES = new Set([
    "AltLeft",
    "AltRight",
    "ControlLeft",
    "ControlRight",
    "ShiftLeft",
    "ShiftRight",
    "MetaLeft",
    "MetaRight"
]);
/**
 * A chord is safe only with Alt or Meta held. Ctrl-only chords are rejected:
 * Ctrl+letter is a PTY control byte (Ctrl+C, Ctrl+A is the tmux/screen prefix),
 * and Ctrl+Space is NUL. A bare key would swallow ordinary typing.
 */ function isSafeChord(chord) {
    if (!chord.code || MODIFIER_CODES.has(chord.code)) return false;
    return !!(chord.alt || chord.meta);
}
function parse(raw) {
    if (!raw) return DEFAULT_CONFIG;
    try {
        const v = JSON.parse(raw);
        if (v && v.disabled === true) return {
            enabled: false
        };
        if (v && typeof v.code === "string") {
            const chord = {
                code: v.code,
                alt: v.alt === true,
                ctrl: v.ctrl === true,
                shift: v.shift === true,
                meta: v.meta === true
            };
            if (isSafeChord(chord)) return {
                enabled: true,
                chord
            };
        }
    } catch (e) {
    // fall through to default
    }
    return DEFAULT_CONFIG;
}
let lastRaw;
let lastConfig = DEFAULT_CONFIG;
/** Called on every keydown, so the JSON parse is memoised on the raw stored string. */ function getLeaderConfig() {
    try {
        const raw = typeof localStorage === "undefined" ? null : localStorage.getItem(LEADER_STORAGE_KEY);
        if (raw !== lastRaw) {
            lastRaw = raw;
            lastConfig = parse(raw);
        }
        return lastConfig;
    } catch (e) {
        return DEFAULT_CONFIG;
    }
}
const LEADER_IDLE_KEY = "ssq.leaderIdleMs";
const DEFAULT_LEADER_IDLE_MS = 3000;
/** Idle time after which an open bar closes with no action. Never below the 3 s default. */ function getLeaderIdleMs() {
    try {
        const n = Number(localStorage.getItem(LEADER_IDLE_KEY));
        return Number.isFinite(n) && n >= DEFAULT_LEADER_IDLE_MS ? n : DEFAULT_LEADER_IDLE_MS;
    } catch (e) {
        return DEFAULT_LEADER_IDLE_MS;
    }
}
/** Returns false (and stores nothing) for an unsafe chord. */ function setLeaderConfig(config) {
    if (config.enabled && !isSafeChord(config.chord)) return false;
    try {
        const value = config.enabled ? config.chord : {
            disabled: true
        };
        localStorage.setItem(LEADER_STORAGE_KEY, JSON.stringify(value));
    } catch (e) {
    // storage blocked: still notify so in-memory consumers re-read the default
    }
    if (true) window.dispatchEvent(new Event(LEADER_CONFIG_EVENT));
    return true;
}
function resetLeaderConfig() {
    try {
        localStorage.removeItem(LEADER_STORAGE_KEY);
    } catch (e) {
    // ignore
    }
    if (true) window.dispatchEvent(new Event(LEADER_CONFIG_EVENT));
}
function chordMatches(e, chord) {
    return e.code === chord.code && e.altKey === !!chord.alt && e.ctrlKey === !!chord.ctrl && e.shiftKey === !!chord.shift && e.metaKey === !!chord.meta;
}
function chordFromEvent(e) {
    return {
        code: e.code,
        alt: e.altKey,
        ctrl: e.ctrlKey,
        shift: e.shiftKey,
        meta: e.metaKey
    };
}
function codeLabel(code) {
    if (code.startsWith("Key")) return code.slice(3);
    if (code.startsWith("Digit")) return code.slice(5);
    const named = {
        Semicolon: ";",
        Comma: ",",
        Period: ".",
        Slash: "/",
        Backslash: "\\",
        Quote: "'",
        BracketLeft: "[",
        BracketRight: "]",
        Minus: "-",
        Equal: "=",
        Backquote: "`",
        Space: "Space"
    };
    var _named_code;
    return (_named_code = named[code]) !== null && _named_code !== void 0 ? _named_code : code;
}
function formatChord(chord) {
    return [
        chord.ctrl && "Ctrl",
        chord.alt && "Alt",
        chord.shift && "Shift",
        chord.meta && "Meta",
        codeLabel(chord.code)
    ].filter(Boolean).join("+");
}
function formatLeader(config) {
    return config.enabled ? formatChord(config.chord) : "disabled";
}


/***/ })

}]);
//# sourceMappingURL=7461-5d7dacee75df807a.js.map