(self["webpackChunk_N_E"] = self["webpackChunk_N_E"] || []).push([[7579],{

/***/ 4009:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   H: () => (/* binding */ useBacklogImageAttachment)
/* harmony export */ });
/* unused harmony export MAX_ATTACHMENT_BYTES */
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* harmony import */ var _lib_config__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(51202);
/* __next_internal_client_entry_do_not_use__ MAX_ATTACHMENT_BYTES,useBacklogImageAttachment auto */ 

const MAX_ATTACHMENT_BYTES = 10 * 1024 * 1024; // 10 MB — matches server-side cap
const TOO_LARGE_MESSAGE = "Image is too large (max 10 MB).";
/**
 * Uploads an image to /v1/upload-backlog-attachment and inserts a markdown
 * image reference at the textarea cursor as it is when the upload finishes
 * (not when it started), so typing during the upload is never overwritten.
 */ function useBacklogImageAttachment(param) {
    let { textareaRef, setValue } = param;
    const [error, setError] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(null);
    const [pending, setPending] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(0);
    const isMounted = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(true);
    (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        isMounted.current = true;
        return ()=>{
            isMounted.current = false;
        };
    }, []);
    const upload = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)(async (file)=>{
        if (!file.type.startsWith("image/")) {
            setError("Only image files can be attached.");
            return;
        }
        if (file.size > MAX_ATTACHMENT_BYTES) {
            setError(TOO_LARGE_MESSAGE);
            return;
        }
        setError(null);
        setPending((n)=>n + 1);
        try {
            const formData = new FormData();
            formData.append("file", file);
            const resp = await fetch("".concat((0,_lib_config__WEBPACK_IMPORTED_MODULE_1__/* .getApiBaseUrl */ .h)(), "/v1/upload-backlog-attachment"), {
                method: "POST",
                body: formData
            });
            if (!resp.ok) {
                if (!isMounted.current) return;
                setError(resp.status === 413 ? TOO_LARGE_MESSAGE : resp.status === 415 ? "Unsupported image type — use PNG, JPEG, GIF, or WebP." : "Upload failed.");
                return;
            }
            const data = await resp.json();
            if (!isMounted.current) return;
            const markdown = "![".concat(data.filename, "](").concat(encodeURI("/api/local/serve".concat(data.path)), ")");
            const el = textareaRef.current;
            var _el_selectionStart, _el_value_length;
            const start = Math.min((_el_selectionStart = el === null || el === void 0 ? void 0 : el.selectionStart) !== null && _el_selectionStart !== void 0 ? _el_selectionStart : Infinity, (_el_value_length = el === null || el === void 0 ? void 0 : el.value.length) !== null && _el_value_length !== void 0 ? _el_value_length : Infinity);
            var _el_selectionEnd, _el_value_length1;
            const end = Math.min((_el_selectionEnd = el === null || el === void 0 ? void 0 : el.selectionEnd) !== null && _el_selectionEnd !== void 0 ? _el_selectionEnd : Infinity, (_el_value_length1 = el === null || el === void 0 ? void 0 : el.value.length) !== null && _el_value_length1 !== void 0 ? _el_value_length1 : Infinity);
            setValue((prev)=>prev.slice(0, Math.min(start, prev.length)) + markdown + prev.slice(Math.min(end, prev.length)));
            requestAnimationFrame(()=>{
                var _el_value_length;
                const pos = Math.min(start, (_el_value_length = el === null || el === void 0 ? void 0 : el.value.length) !== null && _el_value_length !== void 0 ? _el_value_length : start) + markdown.length;
                el === null || el === void 0 ? void 0 : el.setSelectionRange(pos, pos);
            });
            el === null || el === void 0 ? void 0 : el.focus();
        } catch (e) {
            if (isMounted.current) setError("Network error — upload failed.");
        } finally{
            if (isMounted.current) setPending((n)=>n - 1);
        }
    }, [
        textareaRef,
        setValue
    ]);
    const onFileInputChange = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)((e)=>{
        var _e_target_files;
        const file = (_e_target_files = e.target.files) === null || _e_target_files === void 0 ? void 0 : _e_target_files[0];
        e.target.value = "";
        if (file) void upload(file);
    }, [
        upload
    ]);
    const onPaste = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)((e)=>{
        const item = Array.from(e.clipboardData.items).find((i)=>i.type.startsWith("image/"));
        const file = item === null || item === void 0 ? void 0 : item.getAsFile();
        if (!file) return;
        e.preventDefault();
        void upload(file);
    }, [
        upload
    ]);
    return {
        error,
        uploading: pending > 0,
        onFileInputChange,
        onPaste
    };
}


/***/ }),

/***/ 17070:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 17810:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 40480:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   AU: () => (/* binding */ formActions),
/* harmony export */   CM: () => (/* binding */ pipelineModeEmptyHintLink),
/* harmony export */   Cd: () => (/* binding */ descriptionTabActive),
/* harmony export */   DT: () => (/* binding */ removeButton),
/* harmony export */   H8: () => (/* binding */ repoSuggestionDismiss),
/* harmony export */   Iv: () => (/* binding */ pipelineModeEmptyHint),
/* harmony export */   Lt: () => (/* binding */ select),
/* harmony export */   OA: () => (/* binding */ descriptionHeader),
/* harmony export */   PC: () => (/* binding */ textarea),
/* harmony export */   Pf: () => (/* binding */ label),
/* harmony export */   RC: () => (/* binding */ submitButton),
/* harmony export */   TQ: () => (/* binding */ acInput),
/* harmony export */   X7: () => (/* binding */ checkboxHint),
/* harmony export */   Y8: () => (/* binding */ fieldGroup),
/* harmony export */   YN: () => (/* binding */ descriptionToolbar),
/* harmony export */   Z_: () => (/* binding */ hiddenFileInput),
/* harmony export */   Zd: () => (/* binding */ form),
/* harmony export */   bK: () => (/* binding */ acRow),
/* harmony export */   bu: () => (/* binding */ twoColumn),
/* harmony export */   c9: () => (/* binding */ addButton),
/* harmony export */   cD: () => (/* binding */ descriptionTab),
/* harmony export */   dG: () => (/* binding */ acSectionHeader),
/* harmony export */   dR: () => (/* binding */ acStatusSelect),
/* harmony export */   g$: () => (/* binding */ attachButton),
/* harmony export */   gJ: () => (/* binding */ errorMessage),
/* harmony export */   hF: () => (/* binding */ input),
/* harmony export */   lG: () => (/* binding */ acList),
/* harmony export */   m8: () => (/* binding */ checkboxInput),
/* harmony export */   mH: () => (/* binding */ checkboxLabel),
/* harmony export */   mw: () => (/* binding */ required),
/* harmony export */   oL: () => (/* binding */ acSection),
/* harmony export */   qs: () => (/* binding */ previewEmpty),
/* harmony export */   u_: () => (/* binding */ previewBox),
/* harmony export */   v8: () => (/* binding */ repoSuggestionChip),
/* harmony export */   x7: () => (/* binding */ cancelButton),
/* harmony export */   yc: () => (/* binding */ checkboxRow)
/* harmony export */ });
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(65916);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D__WEBPACK_IMPORTED_MODULE_0__);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2FBacklogItemForm_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_1YW2_jOhB_z6_wy5E2D1SQNr3Ql_NPVg4M4K2xWds0JEf970cYSG3uCXRXWq0qNUo8Hs98883Fvvvu7Wl6z7GL_tsgFBKZUXzyUUSheN0g_emERECgCGc_CjjNU1auxDjz0TsW3xznu_cC550433vb183H5q7R6a2gc3eyde60zogz5Uhyho4Jh_1rs34EEieqIxFriYBTLtprnl7qGIwpiZlDFKTSRwEwBWKmtffa2v6z3E9Lez3Bep0SBk5Se_JZyh_08iMJVeIjz3X_KeUzHIaExR3Dztu2_ur8Aw7eYsFzFnYMxCNAVTAfuAhB_MjLCiQ5JWFL7M2QcgQOSS47bj5NwGAENMIpoaeOa_9agueqxMpHjDMof1ACM0kqitUmaF_Qe_ftJQIsobKuKA_WmNViB17YQPsRD3Kp4TY1tS2h24vCBIf86CNX_2yoo3f07Z1gp9RHEDCaQhihDSBvW_lwJjFBBQ6wOhSIBp_VtK2CWfnt2sWJcq_y_nyv5Gqjz_pcoYVYJcyNLyjJOmvs8I9_MV4Ta2PfU3xiv6zHnba7RxxrMV78eK6JdGUn_v8RoH5cn9bqS2Gy_zenGvJnfSfjyhfIRvO72UYhET5lCIujPAznbksOwsQ1MwnJCpVqKgUE4IARe4qjcVAU0VfsLf68zuSOcsBFEy19oSrjFonW1P4CqO_silItHJCThTwJSPZIYDcA6gjgDMUhx9gbHxKgllKEwqRlAo9NQ1XClP7UpZp_Zvr5SPX1MpTYxuK35Eb_jj8MSTeOazO085o3x26szuOT9srhOmrwjX5ufnUn3Q7sFuEc33OahM4Rt3QqgnrgwLYGqijPfAM0nqspTIRBD25iPXBs8oriOU8kYo5ZpkGRYxDn27JWJDt8tlqTMH97QT0xDLBAaCukIR6ptCNCgSaOTX4VRYKBvVeaF8mgxXa6qh_b1keArr5JY2GFhoBMtRfGh7bZoWGEqgyDYx_TWE6t4CvIlanA7nzU_LVU3xW23aH3_s6RLMuvJd_CkvfM19z8R7FtVYK0h_SCQ_UKimM57hgKiTj9y7vWkT48rBlPIjhNZ2dkuMl0RyTmnwrin3vye0JoSzwvY8WSASO7B88aNefTXpqwmtB8Rs1Sl8rIrponUU5Xr537Li5_LxuuWWWDwDzh8gumxt7V1j7hujoTBoOHgZNuTluOTCbhfbh6lfEonuSHFr3VgeDjV_brB2LarmuZ1BnbvUu17ve5tc_YluvWe4GQ9kx2Eu7_Z3AKlOFHxEFKYksPQXNq6xIHrqKj8dBWlGsQKnekqQPtpFAnnRUJG6b_Xh6ZYOPDS4js1fE_3g_vS86SWruSvPKy7L_PNiI32_sL91GTUTovntAobbO1BOVibCEhBEDZSJf1MICUbfUlw0xjyUqbjVJrZ4N0i0mmQfhsL6jPJhCwsJLX3nWp_BwUPtr41C8_vH5uN_XQ_LibwdAAA_22_7D__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(17070);
/* harmony import */ var _shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2FBacklogItemForm_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_1YW2_jOhB_z6_wy5E2D1SQNr3Ql_NPVg4M4K2xWds0JEf970cYSG3uCXRXWq0qNUo8Hs98883Fvvvu7Wl6z7GL_tsgFBKZUXzyUUSheN0g_emERECgCGc_CjjNU1auxDjz0TsW3xznu_cC550433vb183H5q7R6a2gc3eyde60zogz5Uhyho4Jh_1rs34EEieqIxFriYBTLtprnl7qGIwpiZlDFKTSRwEwBWKmtffa2v6z3E9Lez3Bep0SBk5Se_JZyh_08iMJVeIjz3X_KeUzHIaExR3Dztu2_ur8Aw7eYsFzFnYMxCNAVTAfuAhB_MjLCiQ5JWFL7M2QcgQOSS47bj5NwGAENMIpoaeOa_9agueqxMpHjDMof1ACM0kqitUmaF_Qe_ftJQIsobKuKA_WmNViB17YQPsRD3Kp4TY1tS2h24vCBIf86CNX_2yoo3f07Z1gp9RHEDCaQhihDSBvW_lwJjFBBQ6wOhSIBp_VtK2CWfnt2sWJcq_y_nyv5Gqjz_pcoYVYJcyNLyjJOmvs8I9_MV4Ta2PfU3xiv6zHnba7RxxrMV78eK6JdGUn_v8RoH5cn9bqS2Gy_zenGvJnfSfjyhfIRvO72UYhET5lCIujPAznbksOwsQ1MwnJCpVqKgUE4IARe4qjcVAU0VfsLf68zuSOcsBFEy19oSrjFonW1P4CqO_silItHJCThTwJSPZIYDcA6gjgDMUhx9gbHxKgllKEwqRlAo9NQ1XClP7UpZp_Zvr5SPX1MpTYxuK35Eb_jj8MSTeOazO085o3x26szuOT9srhOmrwjX5ufnUn3Q7sFuEc33OahM4Rt3QqgnrgwLYGqijPfAM0nqspTIRBD25iPXBs8oriOU8kYo5ZpkGRYxDn27JWJDt8tlqTMH97QT0xDLBAaCukIR6ptCNCgSaOTX4VRYKBvVeaF8mgxXa6qh_b1keArr5JY2GFhoBMtRfGh7bZoWGEqgyDYx_TWE6t4CvIlanA7nzU_LVU3xW23aH3_s6RLMuvJd_CkvfM19z8R7FtVYK0h_SCQ_UKimM57hgKiTj9y7vWkT48rBlPIjhNZ2dkuMl0RyTmnwrin3vye0JoSzwvY8WSASO7B88aNefTXpqwmtB8Rs1Sl8rIrponUU5Xr537Li5_LxuuWWWDwDzh8gumxt7V1j7hujoTBoOHgZNuTluOTCbhfbh6lfEonuSHFr3VgeDjV_brB2LarmuZ1BnbvUu17ve5tc_YluvWe4GQ9kx2Eu7_Z3AKlOFHxEFKYksPQXNq6xIHrqKj8dBWlGsQKnekqQPtpFAnnRUJG6b_Xh6ZYOPDS4js1fE_3g_vS86SWruSvPKy7L_PNiI32_sL91GTUTovntAobbO1BOVibCEhBEDZSJf1MICUbfUlw0xjyUqbjVJrZ4N0i0mmQfhsL6jPJhCwsJLX3nWp_BwUPtr41C8_vH5uN_XQ_LibwdAAA_22_7D__WEBPACK_IMPORTED_MODULE_1___default = /*#__PURE__*/__webpack_require__.n(_shared_node_modules_69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1_node_modules_bun_vanilla_extract_webpack_plugin_2_3_26_db3e8d9ee620131b_node_modules_vanilla_extract_webpack_plugin_vanilla_virtual_css_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2FBacklogItemForm_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_1YW2_jOhB_z6_wy5E2D1SQNr3Ql_NPVg4M4K2xWds0JEf970cYSG3uCXRXWq0qNUo8Hs98883Fvvvu7Wl6z7GL_tsgFBKZUXzyUUSheN0g_emERECgCGc_CjjNU1auxDjz0TsW3xznu_cC550433vb183H5q7R6a2gc3eyde60zogz5Uhyho4Jh_1rs34EEieqIxFriYBTLtprnl7qGIwpiZlDFKTSRwEwBWKmtffa2v6z3E9Lez3Bep0SBk5Se_JZyh_08iMJVeIjz3X_KeUzHIaExR3Dztu2_ur8Aw7eYsFzFnYMxCNAVTAfuAhB_MjLCiQ5JWFL7M2QcgQOSS47bj5NwGAENMIpoaeOa_9agueqxMpHjDMof1ACM0kqitUmaF_Qe_ftJQIsobKuKA_WmNViB17YQPsRD3Kp4TY1tS2h24vCBIf86CNX_2yoo3f07Z1gp9RHEDCaQhihDSBvW_lwJjFBBQ6wOhSIBp_VtK2CWfnt2sWJcq_y_nyv5Gqjz_pcoYVYJcyNLyjJOmvs8I9_MV4Ta2PfU3xiv6zHnba7RxxrMV78eK6JdGUn_v8RoH5cn9bqS2Gy_zenGvJnfSfjyhfIRvO72UYhET5lCIujPAznbksOwsQ1MwnJCpVqKgUE4IARe4qjcVAU0VfsLf68zuSOcsBFEy19oSrjFonW1P4CqO_silItHJCThTwJSPZIYDcA6gjgDMUhx9gbHxKgllKEwqRlAo9NQ1XClP7UpZp_Zvr5SPX1MpTYxuK35Eb_jj8MSTeOazO085o3x26szuOT9srhOmrwjX5ufnUn3Q7sFuEc33OahM4Rt3QqgnrgwLYGqijPfAM0nqspTIRBD25iPXBs8oriOU8kYo5ZpkGRYxDn27JWJDt8tlqTMH97QT0xDLBAaCukIR6ptCNCgSaOTX4VRYKBvVeaF8mgxXa6qh_b1keArr5JY2GFhoBMtRfGh7bZoWGEqgyDYx_TWE6t4CvIlanA7nzU_LVU3xW23aH3_s6RLMuvJd_CkvfM19z8R7FtVYK0h_SCQ_UKimM57hgKiTj9y7vWkT48rBlPIjhNZ2dkuMl0RyTmnwrin3vye0JoSzwvY8WSASO7B88aNefTXpqwmtB8Rs1Sl8rIrponUU5Xr537Li5_LxuuWWWDwDzh8gumxt7V1j7hujoTBoOHgZNuTluOTCbhfbh6lfEonuSHFr3VgeDjV_brB2LarmuZ1BnbvUu17ve5tc_YluvWe4GQ9kx2Eu7_Z3AKlOFHxEFKYksPQXNq6xIHrqKj8dBWlGsQKnekqQPtpFAnnRUJG6b_Xh6ZYOPDS4js1fE_3g_vS86SWruSvPKy7L_PNiI32_sL91GTUTovntAobbO1BOVibCEhBEDZSJf1MICUbfUlw0xjyUqbjVJrZ4N0i0mmQfhsL6jPJhCwsJLX3nWp_BwUPtr41C8_vH5uN_XQ_LibwdAAA_22_7D__WEBPACK_IMPORTED_MODULE_1__);


var acInput = '_15lm3oah';
var acList = '_15lm3oaf';
var acRow = '_15lm3oag';
var acSection = '_15lm3oad';
var acSectionHeader = '_15lm3oae';
var acStatusSelect = '_15lm3oai';
var addButton = '_15lm3oak';
var attachButton = '_15lm3oat';
var cancelButton = '_15lm3oan';
var checkboxHint = '_15lm3oaa';
var checkboxInput = '_15lm3oa8';
var checkboxLabel = '_15lm3oa9';
var checkboxRow = '_15lm3oa7';
var descriptionHeader = '_15lm3oap';
var descriptionTab = '_15lm3oar';
var descriptionTabActive = '_15lm3oas _15lm3oar';
var descriptionToolbar = '_15lm3oaq';
var errorMessage = '_15lm3oao';
var fieldGroup = '_15lm3oa1';
var form = '_15lm3oa0';
var formActions = '_15lm3oal';
var hiddenFileInput = '_15lm3oau';
var input = '_15lm3oa4';
var label = '_15lm3oa2';
var pipelineModeEmptyHint = '_15lm3oab';
var pipelineModeEmptyHintLink = '_15lm3oac';
var previewBox = '_15lm3oav';
var previewEmpty = '_15lm3oaw';
var removeButton = '_15lm3oaj';
var repoSuggestionChip = '_15lm3oay';
var repoSuggestionDismiss = '_15lm3oaz';
var required = '_15lm3oa3';
var select = '_15lm3oa6';
var submitButton = '_15lm3oam';
var textarea = '_15lm3oa5';
var twoColumn = '_15lm3oax';

/***/ }),

/***/ 67579:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
// ESM COMPAT FLAG
__webpack_require__.r(__webpack_exports__);

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  BacklogComposeWidget: () => (/* binding */ BacklogComposeWidget)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ./src/lib/hooks/useBacklogImageAttachment.ts
var useBacklogImageAttachment = __webpack_require__(4009);
// EXTERNAL MODULE: ./src/components/backlog/BacklogItemForm.css.ts
var BacklogItemForm_css = __webpack_require__(40480);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fbacklog%2FBacklogItemForm.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-1YW2-jOhB-z6_wy5E2D1SQNr3Ql_NPVg4M4K2xWds0JEf970cYSG3uCXRXWq0qNUo8Hs98883Fvvvu7Wl6z7GL_tsgFBKZUXzyUUSheN0g_emERECgCGc-CjjNU1auxDjz0TsW3xznu_cC550433vb183H5q7R6a2gc3eyde60zogz5Uhyho4Jh-1rs34EEieqIxFriYBTLtprnl7qGIwpiZlDFKTSRwEwBWKmtffa2v6z3E9Lez3Bep0SBk5Se-JZyh-08iMJVeIjz3X_KeUzHIaExR3Dztu2_ur8Aw7eYsFzFnYMxCNAVTAfuAhB-MjLCiQ5JWFL7M2QcgQOSS47bj5NwGAENMIpoaeOa-9agueqxMpHjDMof1ACM0kqitUmaF-Qe-ftJQIsobKuKA_WmNViB17YQPsRD3Kp4TY1tS2h24vCBIf86CNX_-2yoo3f07Z1gp9RHEDCaQhihDSBvW__lwJjFBBQ6wOhSIBp-VtK2CWfnt2sWJcq-y-nyv5Gqjz-pcoYVYJcyNLyjJOmvs8I9-MV4Ta2PfU3xiv6zHnba7RxxrMV78eK6JdGUn_v8RoH5cn9bqS2Gy-zenGvJnfSfjyhfIRvO72UYhET5lCIujPAznbksOwsQ1MwnJCpVqKgUE4IARe4qjcVAU0VfsLf68zuSOcsBFEy19oSrjFonW1P4CqO_silItHJCThTwJSPZIYDcA6gjgDMUhx9gbHxKgllKEwqRlAo9NQ1XClP7UpZp-Zvr5SPX1MpTYxuK35Eb_jj8MSTeOazO085o3x26szuOT9srhOmrwjX5ufnUn3Q7sFuEc33OahM4Rt3QqgnrgwLYGqijPfAM0nqspTIRBD25iPXBs8oriOU8kYo5ZpkGRYxDn27JWJDt8tlqTMH97QT0xDLBAaCukIR6ptCNCgSaOTX4VRYKBvVeaF8mgxXa6qh_b1keArr5JY2GFhoBMtRfGh7bZoWGEqgyDYx_TWE6t4CvIlanA7nzU-LVU3xW23aH3_s6RLMuvJd_CkvfM19z8R7FtVYK0h-SCQ-UKimM57hgKiTj9y7vWkT48rBlPIjhNZ2dkuMl0RyTmnwrin3vye0JoSzwvY8WSASO7B88aNefTXpqwmtB8Rs1Sl8rIrponUU5Xr537Li5-LxuuWWWDwDzh8gumxt7V1j7hujoTBoOHgZNuTluOTCbhfbh6lfEonuSHFr3VgeDjV_brB2LarmuZ1BnbvUu17ve5tc_YluvWe4GQ9kx2Eu7-Z3AKlOFHxEFKYksPQXNq6xIHrqKj8dBWlGsQKnekqQPtpFAnnRUJG6b-Xh6ZYOPDS4js1fE-3g-vS86SWruSvPKy7L-PNiI32-sL91GTUTovntAobbO1BOVibCEhBEDZSJf1MICUbfUlw0xjyUqbjVJrZ4N0i0mmQfhsL6jPJhCwsJLX3nWp-BwUPtr41C8-vH5uN_XQ-LibwdAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2FBacklogItemForm_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_1YW2_jOhB_z6_wy5E2D1SQNr3Ql_NPVg4M4K2xWds0JEf970cYSG3uCXRXWq0qNUo8Hs98883Fvvvu7Wl6z7GL_tsgFBKZUXzyUUSheN0g_emERECgCGc_CjjNU1auxDjz0TsW3xznu_cC550433vb183H5q7R6a2gc3eyde60zogz5Uhyho4Jh_1rs34EEieqIxFriYBTLtprnl7qGIwpiZlDFKTSRwEwBWKmtffa2v6z3E9Lez3Bep0SBk5Se_JZyh_08iMJVeIjz3X_KeUzHIaExR3Dztu2_ur8Aw7eYsFzFnYMxCNAVTAfuAhB_MjLCiQ5JWFL7M2QcgQOSS47bj5NwGAENMIpoaeOa_9agueqxMpHjDMof1ACM0kqitUmaF_Qe_ftJQIsobKuKA_WmNViB17YQPsRD3Kp4TY1tS2h24vCBIf86CNX_2yoo3f07Z1gp9RHEDCaQhihDSBvW_lwJjFBBQ6wOhSIBp_VtK2CWfnt2sWJcq_y_nyv5Gqjz_pcoYVYJcyNLyjJOmvs8I9_MV4Ta2PfU3xiv6zHnba7RxxrMV78eK6JdGUn_v8RoH5cn9bqS2Gy_zenGvJnfSfjyhfIRvO72UYhET5lCIujPAznbksOwsQ1MwnJCpVqKgUE4IARe4qjcVAU0VfsLf68zuSOcsBFEy19oSrjFonW1P4CqO_silItHJCThTwJSPZIYDcA6gjgDMUhx9gbHxKgllKEwqRlAo9NQ1XClP7UpZp_Zvr5SPX1MpTYxuK35Eb_jj8MSTeOazO085o3x26szuOT9srhOmrwjX5ufnUn3Q7sFuEc33OahM4Rt3QqgnrgwLYGqijPfAM0nqspTIRBD25iPXBs8oriOU8kYo5ZpkGRYxDn27JWJDt8tlqTMH97QT0xDLBAaCukIR6ptCNCgSaOTX4VRYKBvVeaF8mgxXa6qh_b1keArr5JY2GFhoBMtRfGh7bZoWGEqgyDYx_TWE6t4CvIlanA7nzU_LVU3xW23aH3_s6RLMuvJd_CkvfM19z8R7FtVYK0h_SCQ_UKimM57hgKiTj9y7vWkT48rBlPIjhNZ2dkuMl0RyTmnwrin3vye0JoSzwvY8WSASO7B88aNefTXpqwmtB8Rs1Sl8rIrponUU5Xr537Li5_LxuuWWWDwDzh8gumxt7V1j7hujoTBoOHgZNuTluOTCbhfbh6lfEonuSHFr3VgeDjV_brB2LarmuZ1BnbvUu17ve5tc_YluvWe4GQ9kx2Eu7_Z3AKlOFHxEFKYksPQXNq6xIHrqKj8dBWlGsQKnekqQPtpFAnnRUJG6b_Xh6ZYOPDS4js1fE_3g_vS86SWruSvPKy7L_PNiI32_sL91GTUTovntAobbO1BOVibCEhBEDZSJf1MICUbfUlw0xjyUqbjVJrZ4N0i0mmQfhsL6jPJhCwsJLX3nWp_BwUPtr41C8_vH5uN_XQ_LibwdAAA_22_7D = __webpack_require__(17070);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fbacklog%2FBacklogComposeWidget.css.ts.vanilla.css%22%2C%22source%22%3A%22Ll8xdDE3aW5rMCB7CiAgZGlzcGxheTogZmxleDsKICBmbGV4LWRpcmVjdGlvbjogY29sdW1uOwogIGdhcDogdmFyKC0tXzE5ZXoycnozMCk7CiAgcGFkZGluZzogdmFyKC0tXzE5ZXoycnozMSk7Cn0KLl8xdDE3aW5rMSB7CiAgbWFyZ2luOiAwOwogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYyk7CiAgZm9udC13ZWlnaHQ6IHZhcigtLV8xOWV6MnJ6M2cpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyejApOwp9Ci5fMXQxN2luazIgewogIG1pbi1oZWlnaHQ6IDQ0cHg7CiAgbWluLXdpZHRoOiA0NHB4Owp9Ci5fMXQxN2luazMgewogIG1pbi1oZWlnaHQ6IDE2MHB4OwogIHJlc2l6ZTogdmVydGljYWw7Cn0KLl8xdDE3aW5rNCB7CiAgZGlzcGxheTogZmxleDsKICBmbGV4LXdyYXA6IHdyYXA7CiAgYWxpZ24taXRlbXM6IGNlbnRlcjsKICBnYXA6IHZhcigtLV8xOWV6MnJ6MnopOwp9Ci5fMXQxN2luazUgewogIGZsZXg6IDE7Cn0KLl8xdDE3aW5rNiB7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MCk7Cn0%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fbacklog_2FBacklogComposeWidget_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll8xdDE3aW5rMCB7CiAgZGlzcGxheTogZmxleDsKICBmbGV4LWRpcmVjdGlvbjogY29sdW1uOwogIGdhcDogdmFyKC0tXzE5ZXoycnozMCk7CiAgcGFkZGluZzogdmFyKC0tXzE5ZXoycnozMSk7Cn0KLl8xdDE3aW5rMSB7CiAgbWFyZ2luOiAwOwogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYyk7CiAgZm9udC13ZWlnaHQ6IHZhcigtLV8xOWV6MnJ6M2cpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyejApOwp9Ci5fMXQxN2luazIgewogIG1pbi1oZWlnaHQ6IDQ0cHg7CiAgbWluLXdpZHRoOiA0NHB4Owp9Ci5fMXQxN2luazMgewogIG1pbi1oZWlnaHQ6IDE2MHB4OwogIHJlc2l6ZTogdmVydGljYWw7Cn0KLl8xdDE3aW5rNCB7CiAgZGlzcGxheTogZmxleDsKICBmbGV4LXdyYXA6IHdyYXA7CiAgYWxpZ24taXRlbXM6IGNlbnRlcjsKICBnYXA6IHZhcigtLV8xOWV6MnJ6MnopOwp9Ci5fMXQxN2luazUgewogIGZsZXg6IDE7Cn0KLl8xdDE3aW5rNiB7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MCk7Cn0_22_7D = __webpack_require__(17810);
;// ./src/components/backlog/BacklogComposeWidget.css.ts



var actions = '_1t17ink4';
var attachButton = '_1t17ink6 _15lm3oat _1t17ink2';
var root = '_1t17ink0';
var spacer = '_1t17ink5';
var BacklogComposeWidget_css_textarea = '_1t17ink3';
var title = '_1t17ink1';
var touchTarget = '_1t17ink2';
;// ./src/components/backlog/BacklogComposeWidget.tsx
/* __next_internal_client_entry_do_not_use__ BacklogComposeWidget auto */ 
// +feature: backlog:compose-widget




function BacklogComposeWidget(param) {
    let { initialText, onSubmit, onCancel } = param;
    const [text, setText] = (0,react.useState)(initialText);
    const textareaRef = (0,react.useRef)(null);
    const fileInputRef = (0,react.useRef)(null);
    const { error, uploading, onFileInputChange, onPaste } = (0,useBacklogImageAttachment/* useBacklogImageAttachment */.H)({
        textareaRef,
        setValue: setText
    });
    const canSubmit = text.trim() !== "" && !uploading;
    const submit = ()=>{
        if (canSubmit) onSubmit(text.trim());
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: root,
        "data-testid": "backlog-compose-widget",
        onKeyDown: (e)=>{
            if (e.key === "Escape") {
                e.stopPropagation();
                onCancel(text);
            } else if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
                e.preventDefault();
                submit();
            }
        },
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                id: "backlog-compose-title",
                className: title,
                children: "Create backlog item"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                htmlFor: "backlog-compose-textarea",
                className: BacklogItemForm_css/* label */.Pf,
                children: "Describe the work"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("textarea", {
                id: "backlog-compose-textarea",
                ref: textareaRef,
                className: "".concat(BacklogItemForm_css/* textarea */.PC, " ").concat(BacklogComposeWidget_css_textarea),
                value: text,
                onChange: (e)=>setText(e.target.value),
                onPaste: onPaste,
                placeholder: "What needs to be done? Paste or attach screenshots. Ctrl/⌘+Enter to create.",
                autoFocus: true,
                "data-testid": "backlog-compose-textarea"
            }),
            error && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: BacklogItemForm_css/* errorMessage */.gJ,
                role: "alert",
                "data-testid": "backlog-compose-error",
                children: error
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: actions,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: attachButton,
                        onClick: ()=>{
                            var _fileInputRef_current;
                            return (_fileInputRef_current = fileInputRef.current) === null || _fileInputRef_current === void 0 ? void 0 : _fileInputRef_current.click();
                        },
                        disabled: uploading,
                        "data-testid": "backlog-compose-attach",
                        children: uploading ? "Uploading…" : /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    "aria-hidden": "true",
                                    children: "\uD83D\uDCCE"
                                }),
                                " Attach image"
                            ]
                        })
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                        ref: fileInputRef,
                        type: "file",
                        accept: "image/*",
                        className: BacklogItemForm_css/* hiddenFileInput */.Z_,
                        onChange: onFileInputChange,
                        "data-testid": "backlog-compose-attach-input"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: spacer
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: "".concat(BacklogItemForm_css/* cancelButton */.x7, " ").concat(touchTarget),
                        onClick: ()=>onCancel(text),
                        "data-testid": "backlog-compose-cancel",
                        children: "Cancel"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: "".concat(BacklogItemForm_css/* submitButton */.RC, " ").concat(touchTarget),
                        onClick: submit,
                        disabled: !canSubmit,
                        "data-testid": "backlog-compose-submit",
                        children: "Create"
                    })
                ]
            })
        ]
    });
}


/***/ })

}]);
//# sourceMappingURL=7579.4ee6742a18cd0322.js.map