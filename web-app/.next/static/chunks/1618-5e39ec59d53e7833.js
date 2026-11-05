"use strict";
(self["webpackChunk_N_E"] = self["webpackChunk_N_E"] || []).push([[1618],{

/***/ 4009:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

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

/***/ 40480:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

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

/***/ 91618:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  T: () => (/* binding */ BacklogItemForm)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-markdown@10.1.0+5bba37e96b694e48/node_modules/react-markdown/lib/index.js + 114 modules
var lib = __webpack_require__(55206);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/remark-gfm@4.0.1+759ce506b1ed1a42/node_modules/remark-gfm/lib/index.js + 56 modules
var remark_gfm_lib = __webpack_require__(69896);
// EXTERNAL MODULE: ./src/lib/nav/index.ts
var nav = __webpack_require__(41078);
// EXTERNAL MODULE: ./src/lib/hooks/useBacklogService.ts
var useBacklogService = __webpack_require__(8570);
// EXTERNAL MODULE: ./src/lib/contexts/FeatureFlagsContext.tsx
var FeatureFlagsContext = __webpack_require__(63367);
// EXTERNAL MODULE: ./src/components/ui/RepoPathInput.tsx + 2 modules
var RepoPathInput = __webpack_require__(34609);
// EXTERNAL MODULE: ./src/components/ui/RadioGroup.tsx
var RadioGroup = __webpack_require__(98764);
// EXTERNAL MODULE: ./src/components/ui/RadioGroup.css.ts
var RadioGroup_css = __webpack_require__(95923);
// EXTERNAL MODULE: ./src/lib/github/urlParser.ts
var urlParser = __webpack_require__(11538);
// EXTERNAL MODULE: ./src/lib/hooks/useGitHubEnterpriseHosts.ts
var useGitHubEnterpriseHosts = __webpack_require__(44635);
// EXTERNAL MODULE: ./src/lib/hooks/useBacklogImageAttachment.ts
var useBacklogImageAttachment = __webpack_require__(4009);
// EXTERNAL MODULE: ./src/lib/routes.ts + 1 modules
var routes = __webpack_require__(76514);
;// ./src/lib/backlog/categoryDefaults.ts
// categoryDefaults.ts — hardcoded backlog item category taxonomy and the
// per-category automation-toggle defaults BacklogItemForm.tsx applies once,
// at category-selection time, in create mode only (docs/tasks/backlog-
// feature-improvement.md, 07-27/07-28 updates, recommended action #5).
//
// This is deliberately a small, fixed enum rather than an extensible CRUD
// settings page (unlike PipelineMode) — see
// the `interface-pollution-checklist` skill: a 4-value taxonomy with no
// evidence of a need for custom categories doesn't warrant a
// web-app/src/app/settings/categories/ CRUD page. The server only persists
// and validates the category string itself (session.IsValidBacklogCategory,
// session/domain/backlog.go); it never resolves or applies these defaults —
// applying them is purely a client-side "apply template" action that the
// user is always free to override afterward.
/** A single selectable category option for the category picker. */ /**
 * Slug of the seeded SDD pipeline mode (session.DefaultSDDPipelineModeSlug on
 * the backend, EnsureDefaultSDDPipelineMode in session/pipeline_mode_seed.go)
 * — seeded unconditionally at boot whenever ent storage is available, so this
 * slug is safe to reference here even though it isn't guaranteed to resolve
 * to an *enabled* mode (an operator could disable or delete it later; the
 * form's existing "unknown mode" handling covers that case the same way it
 * already does for a manually-entered stale pipelineMode).
 */ const SDD_PIPELINE_MODE_SLUG = "sdd";
/** The 4-value backlog category taxonomy — keep in sync with
 * session/domain/backlog.go's BacklogCategory constants. */ const BACKLOG_CATEGORIES = [
    {
        value: "bugfix",
        label: "Bugfix",
        description: "Well-scoped fix — skip formal planning and spawn immediately."
    },
    {
        value: "feature",
        label: "Feature",
        description: "New capability — full rigor, run the SDD pipeline."
    },
    {
        value: "chore",
        label: "Chore",
        description: "Low-risk maintenance — fully automated end to end."
    },
    {
        value: "refactor",
        label: "Refactor",
        description: "Needs human judgment — no shortcuts."
    }
];
/**
 * Per-category automation-toggle defaults, keyed by BACKLOG_CATEGORIES'
 * value. Applied as a one-time "apply template" action — see
 * BacklogItemForm.tsx's handleCategoryChange — not a live binding; every
 * field stays fully editable afterward and a manual override always wins.
 */ const CATEGORY_DEFAULTS = {
    bugfix: {
        // Fast path — bugs are usually well-scoped, skip formal planning and
        // spawn immediately. autoApprovePlan is moot here (skipPlanning already
        // bypasses the plan-approval gate entirely) but set false for clarity.
        autoSpawnSession: true,
        skipPlanning: true,
        skipReviewGate: false,
        autoCreatePR: false,
        autoApprovePlan: false,
        pipelineMode: ""
    },
    feature: {
        // Full rigor — use the SDD pipeline mode, human reviews the plan.
        autoSpawnSession: false,
        skipPlanning: false,
        skipReviewGate: false,
        autoCreatePR: false,
        autoApprovePlan: false,
        pipelineMode: SDD_PIPELINE_MODE_SLUG
    },
    chore: {
        // Low-risk, fully automated — no manual gate, including plan approval.
        autoSpawnSession: true,
        skipPlanning: true,
        skipReviewGate: true,
        autoCreatePR: true,
        autoApprovePlan: true,
        pipelineMode: ""
    },
    refactor: {
        // Needs human judgment, no shortcuts.
        autoSpawnSession: false,
        skipPlanning: false,
        skipReviewGate: false,
        autoCreatePR: false,
        autoApprovePlan: false,
        pipelineMode: ""
    }
};

// EXTERNAL MODULE: ./src/components/backlog/BacklogItemForm.css.ts
var BacklogItemForm_css = __webpack_require__(40480);
// EXTERNAL MODULE: ./src/components/backlog/markdownBody.css.ts
var markdownBody_css = __webpack_require__(14613);
;// ./src/components/backlog/BacklogItemForm.tsx
/* __next_internal_client_entry_do_not_use__ BacklogItemForm auto */ 
// +feature: backlog:item-form
















// The 9 content-template fields on PipelineMode — used to detect whether the
// selected mode's rendered content depends on {{repo_path}} (G-1).
const CONTENT_TEMPLATE_KEYS = [
    "statusCommandTemplate",
    "doneCommandTemplate",
    "failCommandTemplate",
    "reviewCommandTemplate",
    "shipCommandTemplate",
    "helpCommandTemplate",
    "triagePromptTemplate",
    "reviewPromptTemplate",
    "initialPromptTemplate"
];
const DEFAULT_PIPELINE_MODE_OPTION = {
    value: "",
    label: "Default",
    description: "Built-in default pipeline",
    dataTestId: "backlog-pipeline-mode-default"
};
const PIPELINE_MODE_FETCH_ERROR_NOTICE = "Couldn't load pipeline modes — you can still save with Default.";
const PIPELINE_MODE_UNKNOWN_HINT = "This item references a pipeline mode that no longer exists or is disabled. Choosing a different mode below will replace it when you save.";
// Slug of the seeded SDD pipeline mode (session.DefaultSDDPipelineModeSlug on
// the backend) and the feature flag name (server/services/feature_flag_service.go's
// sddDefaultPipelineFlagName) gating whether a brand-new item pre-selects it.
// See project_plans/backlog-sdd-default-pipeline — this pre-selection is a
// default, not a lock: the user can still pick any other mode before saving.
const BacklogItemForm_SDD_PIPELINE_MODE_SLUG = "sdd";
const SDD_DEFAULT_PIPELINE_FLAG = "backlog:sdd-default-pipeline";
const UNCATEGORIZED_OPTION = {
    value: "",
    label: "Uncategorized",
    description: "No automation defaults applied — set overrides manually below.",
    dataTestId: "backlog-category-uncategorized"
};
const AC_STATUS_OPTIONS = [
    {
        value: "pending",
        label: "Pending"
    },
    {
        value: "in_progress",
        label: "In Progress"
    },
    {
        value: "done",
        label: "Done"
    }
];
function BacklogItemForm(param) {
    let { initialValues, onSubmit, onCancel, isLoading = false, autoSuggestedRepoPath } = param;
    var _initialValues_title;
    const [title, setTitle] = (0,react.useState)((_initialValues_title = initialValues === null || initialValues === void 0 ? void 0 : initialValues.title) !== null && _initialValues_title !== void 0 ? _initialValues_title : "");
    var _initialValues_description;
    const [description, setDescription] = (0,react.useState)((_initialValues_description = initialValues === null || initialValues === void 0 ? void 0 : initialValues.description) !== null && _initialValues_description !== void 0 ? _initialValues_description : "");
    var _initialValues_repoPath;
    const [repoPath, setRepoPath] = (0,react.useState)((_initialValues_repoPath = initialValues === null || initialValues === void 0 ? void 0 : initialValues.repoPath) !== null && _initialValues_repoPath !== void 0 ? _initialValues_repoPath : "");
    const [suggestionDismissed, setSuggestionDismissed] = (0,react.useState)(false);
    var _initialValues_priority;
    const [priority, setPriority] = (0,react.useState)((_initialValues_priority = initialValues === null || initialValues === void 0 ? void 0 : initialValues.priority) !== null && _initialValues_priority !== void 0 ? _initialValues_priority : 3);
    var _initialValues_skipPlanning;
    const [skipPlanning, setSkipPlanning] = (0,react.useState)((_initialValues_skipPlanning = initialValues === null || initialValues === void 0 ? void 0 : initialValues.skipPlanning) !== null && _initialValues_skipPlanning !== void 0 ? _initialValues_skipPlanning : false);
    var _initialValues_skipReviewGate;
    const [skipReviewGate, setSkipReviewGate] = (0,react.useState)((_initialValues_skipReviewGate = initialValues === null || initialValues === void 0 ? void 0 : initialValues.skipReviewGate) !== null && _initialValues_skipReviewGate !== void 0 ? _initialValues_skipReviewGate : false);
    var _initialValues_autoSpawnSession;
    const [autoSpawnSession, setAutoSpawnSession] = (0,react.useState)((_initialValues_autoSpawnSession = initialValues === null || initialValues === void 0 ? void 0 : initialValues.autoSpawnSession) !== null && _initialValues_autoSpawnSession !== void 0 ? _initialValues_autoSpawnSession : false);
    var _initialValues_autoCreatePR;
    const [autoCreatePR, setAutoCreatePR] = (0,react.useState)((_initialValues_autoCreatePR = initialValues === null || initialValues === void 0 ? void 0 : initialValues.autoCreatePR) !== null && _initialValues_autoCreatePR !== void 0 ? _initialValues_autoCreatePR : false);
    var _initialValues_autoApprovePlan;
    const [autoApprovePlan, setAutoApprovePlan] = (0,react.useState)((_initialValues_autoApprovePlan = initialValues === null || initialValues === void 0 ? void 0 : initialValues.autoApprovePlan) !== null && _initialValues_autoApprovePlan !== void 0 ? _initialValues_autoApprovePlan : false);
    var _initialValues_acCriteria;
    const [acCriteria, setAcCriteria] = (0,react.useState)((_initialValues_acCriteria = initialValues === null || initialValues === void 0 ? void 0 : initialValues.acCriteria) !== null && _initialValues_acCriteria !== void 0 ? _initialValues_acCriteria : []);
    var _initialValues_pipelineMode;
    const [pipelineMode, setPipelineMode] = (0,react.useState)((_initialValues_pipelineMode = initialValues === null || initialValues === void 0 ? void 0 : initialValues.pipelineMode) !== null && _initialValues_pipelineMode !== void 0 ? _initialValues_pipelineMode : "");
    var _initialValues_category;
    const [category, setCategory] = (0,react.useState)((_initialValues_category = initialValues === null || initialValues === void 0 ? void 0 : initialValues.category) !== null && _initialValues_category !== void 0 ? _initialValues_category : "");
    const [costBudgetThreshold, setCostBudgetThreshold] = (0,react.useState)((initialValues === null || initialValues === void 0 ? void 0 : initialValues.costBudgetThresholdUsd) !== undefined ? String(initialValues.costBudgetThresholdUsd) : "");
    // Same presence-gating rationale as categoryTouchedRef above: only send
    // cost_budget_threshold_usd on an existing item's Update when the operator
    // actually touched this session's input, so a possibly-stale
    // initialValues.costBudgetThresholdUsd never overwrites the item's real
    // stored threshold with itself (a no-op resend is harmless, but resending a
    // stale value is not).
    const costBudgetThresholdTouchedRef = (0,react.useRef)(false);
    // Guards the one-shot SDD default pre-selection below from ever re-firing
    // after either the user has manually touched the selector, or the
    // pre-selection has already applied once — see handlePipelineModeChange
    // and the effect below.
    const pipelineModeTouchedRef = (0,react.useRef)(!!(initialValues === null || initialValues === void 0 ? void 0 : initialValues.id));
    // Edit-mode-only guard against submitting a stale `category` (see
    // handleCategoryChange and handleSubmit below): initialValues.category can
    // lag the item's real, server-side category if the caller passed a
    // not-yet-reconciled snapshot (BacklogItemDetail.tsx's `item` can be
    // populated from the shared live-item store before its own authoritative
    // fetch resolves). Only an explicit click on the category selector — never
    // this state's initial mount value — sets this true.
    const categoryTouchedRef = (0,react.useRef)(false);
    const [errors, setErrors] = (0,react.useState)({});
    const [submitting, setSubmitting] = (0,react.useState)(false);
    const [descriptionTab, setDescriptionTab] = (0,react.useState)("write");
    const descriptionRef = (0,react.useRef)(null);
    const { error: attachmentError, uploading, onFileInputChange: handleFileInputChange, onPaste: handleDescriptionPaste } = (0,useBacklogImageAttachment/* useBacklogImageAttachment */.H)({
        textareaRef: descriptionRef,
        setValue: setDescription
    });
    const fileInputRef = (0,react.useRef)(null);
    const { listPipelineModes } = (0,useBacklogService/* useBacklogService */.gG)();
    const [availableModes, setAvailableModes] = (0,react.useState)([]);
    const [modesLoading, setModesLoading] = (0,react.useState)(true);
    const [modesError, setModesError] = (0,react.useState)(false);
    // Story 3.2.1 / G-4 / G-3: fetch enabled pipeline modes on mount. While
    // pending or on failure, the RadioGroup below falls back to only the
    // hardcoded "Default" option — never blocking the form.
    (0,react.useEffect)(()=>{
        let cancelled = false;
        setModesLoading(true);
        setModesError(false);
        listPipelineModes().then((modes)=>{
            if (cancelled) return;
            setAvailableModes(modes.filter((m)=>m.enabled));
        }).catch(()=>{
            if (cancelled) return;
            setModesError(true);
        }).finally(()=>{
            if (!cancelled) setModesLoading(false);
        });
        return ()=>{
            cancelled = true;
        };
    }, [
        listPipelineModes
    ]);
    // Pre-select the seeded "sdd" pipeline mode for a brand-new item once the
    // operator has opted in via the backlog:sdd-default-pipeline feature flag
    // (see project_plans/backlog-sdd-default-pipeline). This is purely a
    // default: pipelineModeTouchedRef.current stays false only until this
    // effect fires or the user picks a value themselves (handlePipelineModeChange
    // below), whichever happens first — after that, this effect never touches
    // pipelineMode again, so it can never fight a manual choice, and it never
    // runs at all for an existing item (pipelineModeTouchedRef starts true
    // whenever initialValues?.id is set).
    const sddDefaultFlagEnabled = (0,FeatureFlagsContext/* useFeatureFlag */.u)(SDD_DEFAULT_PIPELINE_FLAG);
    (0,react.useEffect)(()=>{
        if (pipelineModeTouchedRef.current) return;
        if (modesLoading || modesError) return;
        if (!sddDefaultFlagEnabled) return;
        if (!availableModes.some((m)=>m.slug === BacklogItemForm_SDD_PIPELINE_MODE_SLUG)) return;
        pipelineModeTouchedRef.current = true;
        setPipelineMode(BacklogItemForm_SDD_PIPELINE_MODE_SLUG);
    }, [
        sddDefaultFlagEnabled,
        modesLoading,
        modesError,
        availableModes
    ]);
    const handlePipelineModeChange = (0,react.useCallback)((value)=>{
        pipelineModeTouchedRef.current = true;
        setPipelineMode(value);
    }, []);
    const categoryOptions = (0,react.useMemo)(()=>[
            UNCATEGORIZED_OPTION,
            ...BACKLOG_CATEGORIES.map((c)=>({
                    value: c.value,
                    label: c.label,
                    description: c.description,
                    dataTestId: "backlog-category-".concat(c.value)
                }))
        ], []);
    const categoryHintForValue = (0,react.useCallback)((v)=>{
        var _categoryOptions_find;
        return (_categoryOptions_find = categoryOptions.find((o)=>o.value === v)) === null || _categoryOptions_find === void 0 ? void 0 : _categoryOptions_find.description;
    }, [
        categoryOptions
    ]);
    // Applies a category's automation-toggle defaults to local form state.
    // Deliberately scoped to CREATE mode only (initialValues?.id unset) — this
    // is a one-time "apply template" action fired at category-selection time,
    // never a persistent binding: every field it touches remains a normal,
    // fully-editable control afterward, and any later manual change to that
    // field always wins over what this handler set. Re-selecting a category
    // (including switching between two categories) re-applies that category's
    // defaults from scratch each time — deliberately chosen over "don't
    // clobber a field the user already touched" for predictability: picking a
    // category always shows exactly that category's defaults, with no
    // per-field history to reason about. On an existing item (edit mode) this
    // only ever updates the `category` label itself — the automation toggles
    // are intentionally left untouched.
    const handleCategoryChange = (0,react.useCallback)((value)=>{
        categoryTouchedRef.current = true;
        setCategory(value);
        if (initialValues === null || initialValues === void 0 ? void 0 : initialValues.id) return;
        const defaults = CATEGORY_DEFAULTS[value];
        if (!defaults) return;
        setSkipReviewGate(defaults.skipReviewGate);
        setSkipPlanning(defaults.skipPlanning);
        setAutoSpawnSession(defaults.autoSpawnSession);
        setAutoCreatePR(defaults.autoCreatePR);
        setAutoApprovePlan(defaults.autoApprovePlan);
        pipelineModeTouchedRef.current = true;
        setPipelineMode(defaults.pipelineMode);
    }, [
        initialValues === null || initialValues === void 0 ? void 0 : initialValues.id
    ]);
    // Options offered by the RadioGroup: "Default" is always first and always
    // present. While the fetch is pending or failed, no other options render —
    // this alone satisfies G-4/G-3's "only Default selectable" requirement.
    const pipelineModeOptions = (0,react.useMemo)(()=>{
        if (modesLoading || modesError) {
            return [
                DEFAULT_PIPELINE_MODE_OPTION
            ];
        }
        return [
            DEFAULT_PIPELINE_MODE_OPTION,
            ...availableModes.map((m)=>({
                    value: m.slug,
                    label: m.name,
                    description: m.description || undefined,
                    dataTestId: "backlog-pipeline-mode-".concat(m.slug)
                }))
        ];
    }, [
        modesLoading,
        modesError,
        availableModes
    ]);
    // The fetch succeeded but no enabled modes exist yet — distinct from
    // loading/error: without this, a picker with nothing to pick from looks
    // identical to a picker that's broken or hasn't fetched at all (the exact
    // "single greyed Default button, clicking it does nothing" confusion
    // flagged in docs/tasks/backlog-feature-improvement.md's 2026-07-19 audit).
    const hasNoAvailableModes = !modesLoading && !modesError && availableModes.length === 0;
    // G-2: an item's stored pipelineMode may reference a mode that's since been
    // deleted or disabled. Only evaluate once the fetch has actually succeeded —
    // during loading/error we can't yet tell "unresolvable" apart from
    // "haven't checked yet".
    const unresolvedPipelineMode = (0,react.useMemo)(()=>{
        if (modesLoading || modesError) return null;
        if (!pipelineMode) return null;
        if (pipelineModeOptions.some((o)=>o.value === pipelineMode)) return null;
        return pipelineMode;
    }, [
        modesLoading,
        modesError,
        pipelineMode,
        pipelineModeOptions
    ]);
    const pipelineModeHintForValue = (0,react.useCallback)((v)=>{
        var _pipelineModeOptions_find;
        if (unresolvedPipelineMode && v === unresolvedPipelineMode) {
            return PIPELINE_MODE_UNKNOWN_HINT;
        }
        return (_pipelineModeOptions_find = pipelineModeOptions.find((o)=>o.value === v)) === null || _pipelineModeOptions_find === void 0 ? void 0 : _pipelineModeOptions_find.description;
    }, [
        unresolvedPipelineMode,
        pipelineModeOptions
    ]);
    const unresolvedPipelineModeOption = unresolvedPipelineMode ? /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
        type: "button",
        role: "radio",
        "aria-checked": "true",
        "aria-disabled": "true",
        disabled: true,
        tabIndex: -1,
        className: [
            RadioGroup_css/* radioBtn */.U1,
            RadioGroup_css/* radioBtnActive */.UE
        ].join(" "),
        "data-testid": "backlog-pipeline-mode-unknown-".concat(unresolvedPipelineMode),
        children: "Unknown mode ('".concat(unresolvedPipelineMode, "')")
    }) : undefined;
    // G-1: the selected mode's own content-template fields may assume a
    // repo path is present. Non-blocking — warns, never disables.
    const selectedPipelineMode = (0,react.useMemo)(()=>{
        var _availableModes_find;
        return (_availableModes_find = availableModes.find((m)=>m.slug === pipelineMode)) !== null && _availableModes_find !== void 0 ? _availableModes_find : null;
    }, [
        availableModes,
        pipelineMode
    ]);
    const selectedModeRequiresRepoPath = (0,react.useMemo)(()=>{
        if (!selectedPipelineMode) return false;
        return CONTENT_TEMPLATE_KEYS.some((key)=>{
            var _selectedPipelineMode_key;
            return (_selectedPipelineMode_key = selectedPipelineMode[key]) === null || _selectedPipelineMode_key === void 0 ? void 0 : _selectedPipelineMode_key.includes("{{repo_path}}");
        });
    }, [
        selectedPipelineMode
    ]);
    const showRepoPathPrerequisiteWarning = selectedModeRequiresRepoPath && !repoPath.trim();
    const validate = (0,react.useCallback)(()=>{
        const errs = {};
        if (!title.trim()) {
            errs.title = "Title is required.";
        }
        if (!(initialValues === null || initialValues === void 0 ? void 0 : initialValues.id) && !repoPath.trim()) {
            errs.repoPath = "Repository path is required for automated triage.";
        }
        const trimmedThreshold = costBudgetThreshold.trim();
        if (trimmedThreshold !== "") {
            const parsed = Number(trimmedThreshold);
            if (isNaN(parsed) || parsed < 0) {
                errs.costBudgetThreshold = "Budget threshold must be zero or greater.";
            }
        }
        return errs;
    }, [
        title,
        repoPath,
        initialValues === null || initialValues === void 0 ? void 0 : initialValues.id,
        costBudgetThreshold
    ]);
    const handleSubmit = (0,react.useCallback)(async (e)=>{
        e.preventDefault();
        const errs = validate();
        if (Object.keys(errs).length > 0) {
            setErrors(errs);
            return;
        }
        setErrors({});
        setSubmitting(true);
        try {
            // Evaluate vagueness before submitting: short description + no AC = vague
            const descriptionText = description.trim();
            const isVague = descriptionText.length < 80 && acCriteria.length === 0;
            // On an existing item, only send `category` if the user actually
            // touched the selector this session. `category`'s local state is
            // seeded once from initialValues.category at mount, which can be
            // stale (see categoryTouchedRef's comment above); omitting an
            // untouched field lets the server's presence-gated partial-update
            // semantics (session.v1.UpdateBacklogItemRequest.category is an
            // `optional string`) leave the item's real, already-stored category
            // — and thus whatever automation profile it implies — untouched,
            // instead of silently overwriting it with a possibly-wrong value on
            // every save. Create mode has no existing value to protect, so it
            // always sends the current selection (including "" for
            // Uncategorized).
            const categoryForSubmit = !(initialValues === null || initialValues === void 0 ? void 0 : initialValues.id) || categoryTouchedRef.current ? category : undefined;
            // Same "only send if touched (or creating)" gating as categoryForSubmit
            // above — see costBudgetThresholdTouchedRef's comment. An empty input
            // is sent as `undefined` (omitted from the request), which the server
            // treats as "leave the existing threshold untouched," not "clear it" —
            // this control cannot yet clear an already-configured threshold back
            // to unset, only set or change it (same limitation reworkCapOverride's
            // single-pointer-presence convention already has for its own sentinel
            // value).
            const trimmedThreshold = costBudgetThreshold.trim();
            const parsedThreshold = trimmedThreshold === "" ? undefined : Number(trimmedThreshold);
            const costBudgetThresholdForSubmit = !(initialValues === null || initialValues === void 0 ? void 0 : initialValues.id) || costBudgetThresholdTouchedRef.current ? parsedThreshold : undefined;
            await onSubmit({
                title: title.trim(),
                description: descriptionText || undefined,
                repoPath: repoPath.trim(),
                priority,
                skipPlanning,
                skipReviewGate,
                autoSpawnSession,
                autoCreatePR,
                autoApprovePlan,
                acCriteria: acCriteria.map((c, i)=>({
                        ...c,
                        index: i
                    })),
                skipTriage: isVague,
                pipelineMode,
                category: categoryForSubmit,
                costBudgetThresholdUsd: costBudgetThresholdForSubmit
            });
        } finally{
            setSubmitting(false);
        }
    }, [
        title,
        description,
        repoPath,
        priority,
        skipPlanning,
        skipReviewGate,
        autoSpawnSession,
        autoCreatePR,
        autoApprovePlan,
        acCriteria,
        pipelineMode,
        category,
        costBudgetThreshold,
        initialValues === null || initialValues === void 0 ? void 0 : initialValues.id,
        onSubmit,
        validate
    ]);
    const addCriterion = (0,react.useCallback)(()=>{
        setAcCriteria((prev)=>[
                ...prev,
                {
                    index: prev.length,
                    text: "",
                    status: "pending"
                }
            ]);
    }, []);
    const removeCriterion = (0,react.useCallback)((index)=>{
        setAcCriteria((prev)=>prev.filter((_, i)=>i !== index).map((c, i)=>({
                    ...c,
                    index: i
                })));
    }, []);
    const updateCriterionText = (0,react.useCallback)((index, text)=>{
        setAcCriteria((prev)=>prev.map((c, i)=>i === index ? {
                    ...c,
                    text
                } : c));
    }, []);
    const updateCriterionStatus = (0,react.useCallback)((index, status)=>{
        setAcCriteria((prev)=>prev.map((c, i)=>i === index ? {
                    ...c,
                    status
                } : c));
    }, []);
    const busy = submitting || isLoading;
    const { hosts: enterpriseHosts } = (0,useGitHubEnterpriseHosts/* useGitHubEnterpriseHosts */.P)();
    const isCloningRepo = (0,react.useMemo)(()=>(0,urlParser/* isGitHubRef */.Pu)(repoPath, enterpriseHosts), [
        repoPath,
        enterpriseHosts
    ]);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("form", {
        className: BacklogItemForm_css/* form */.Zd,
        onSubmit: handleSubmit,
        "aria-label": "Backlog item form",
        noValidate: true,
        "data-testid": "backlog-item-form",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: BacklogItemForm_css/* fieldGroup */.Y8,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                        htmlFor: "backlog-title",
                        className: BacklogItemForm_css/* label */.Pf,
                        children: [
                            "Title ",
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: BacklogItemForm_css/* required */.mw,
                                "aria-hidden": "true",
                                children: "*"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                        id: "backlog-title",
                        type: "text",
                        className: BacklogItemForm_css/* input */.hF,
                        value: title,
                        onChange: (e)=>setTitle(e.target.value),
                        placeholder: "What needs to be done?",
                        required: true,
                        "aria-required": "true",
                        "aria-invalid": !!errors.title,
                        "aria-describedby": errors.title ? "backlog-title-error" : undefined,
                        disabled: busy,
                        "data-testid": "backlog-title-input"
                    }),
                    errors.title && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        id: "backlog-title-error",
                        className: BacklogItemForm_css/* errorMessage */.gJ,
                        role: "alert",
                        children: errors.title
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: BacklogItemForm_css/* fieldGroup */.Y8,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: BacklogItemForm_css/* descriptionHeader */.OA,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                htmlFor: "backlog-description",
                                className: BacklogItemForm_css/* label */.Pf,
                                children: "Description"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: BacklogItemForm_css/* descriptionToolbar */.YN,
                                role: "tablist",
                                "aria-label": "Description mode",
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        type: "button",
                                        role: "tab",
                                        "aria-selected": descriptionTab === "write",
                                        className: descriptionTab === "write" ? BacklogItemForm_css/* descriptionTabActive */.Cd : BacklogItemForm_css/* descriptionTab */.cD,
                                        onClick: ()=>setDescriptionTab("write"),
                                        "data-testid": "backlog-description-tab-write",
                                        children: "Write"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        type: "button",
                                        role: "tab",
                                        "aria-selected": descriptionTab === "preview",
                                        className: descriptionTab === "preview" ? BacklogItemForm_css/* descriptionTabActive */.Cd : BacklogItemForm_css/* descriptionTab */.cD,
                                        onClick: ()=>setDescriptionTab("preview"),
                                        "data-testid": "backlog-description-tab-preview",
                                        children: "Preview"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        type: "button",
                                        className: BacklogItemForm_css/* attachButton */.g$,
                                        onClick: ()=>{
                                            var _fileInputRef_current;
                                            return (_fileInputRef_current = fileInputRef.current) === null || _fileInputRef_current === void 0 ? void 0 : _fileInputRef_current.click();
                                        },
                                        disabled: busy || uploading,
                                        "data-testid": "backlog-attach-image",
                                        children: uploading ? "Uploading…" : "📎 Attach image"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                        ref: fileInputRef,
                                        type: "file",
                                        accept: "image/*",
                                        className: BacklogItemForm_css/* hiddenFileInput */.Z_,
                                        onChange: handleFileInputChange,
                                        disabled: busy || uploading,
                                        "aria-label": "Attach image",
                                        "data-testid": "backlog-attach-image-input"
                                    })
                                ]
                            })
                        ]
                    }),
                    descriptionTab === "write" ? /*#__PURE__*/ (0,jsx_runtime.jsx)("textarea", {
                        id: "backlog-description",
                        ref: descriptionRef,
                        className: BacklogItemForm_css/* textarea */.PC,
                        value: description,
                        onChange: (e)=>setDescription(e.target.value),
                        onPaste: handleDescriptionPaste,
                        placeholder: "Provide more context (optional). Supports markdown — paste or attach a screenshot.",
                        disabled: busy,
                        "data-testid": "backlog-description-input"
                    }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: BacklogItemForm_css/* previewBox */.u_,
                        "data-testid": "backlog-description-preview",
                        children: description.trim() ? /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                            className: markdownBody_css/* markdownBody */.e,
                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)(lib/* Markdown */.oz, {
                                remarkPlugins: [
                                    remark_gfm_lib/* default */.A
                                ],
                                children: description
                            })
                        }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                            className: BacklogItemForm_css/* previewEmpty */.qs,
                            children: "Nothing to preview yet."
                        })
                    }),
                    attachmentError && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: BacklogItemForm_css/* errorMessage */.gJ,
                        role: "alert",
                        "data-testid": "backlog-attach-image-error",
                        children: attachmentError
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: BacklogItemForm_css/* twoColumn */.bu,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: BacklogItemForm_css/* fieldGroup */.Y8,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                htmlFor: "backlog-repo-path",
                                className: BacklogItemForm_css/* label */.Pf,
                                children: [
                                    "Repository Path ",
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: BacklogItemForm_css/* required */.mw,
                                        "aria-hidden": "true",
                                        children: "*"
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)(RepoPathInput/* RepoPathInput */.E, {
                                id: "backlog-repo-path",
                                value: repoPath,
                                onChange: setRepoPath,
                                placeholder: "/home/user/project or https://github.com/owner/repo",
                                required: true,
                                disabled: busy,
                                error: errors.repoPath,
                                hint: "Local path to your clone, or a GitHub URL — we'll clone it for you.",
                                detectGitHubUrl: true,
                                "data-testid": "backlog-repo-path-input"
                            }),
                            autoSuggestedRepoPath && !suggestionDismissed && repoPath === autoSuggestedRepoPath && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                className: BacklogItemForm_css/* repoSuggestionChip */.v8,
                                "data-testid": "repo-suggestion-chip",
                                children: [
                                    "Suggested from your sessions",
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        type: "button",
                                        className: BacklogItemForm_css/* repoSuggestionDismiss */.H8,
                                        "data-testid": "repo-suggestion-dismiss",
                                        "aria-label": "Dismiss suggested repository",
                                        disabled: busy,
                                        onClick: ()=>{
                                            var _document_getElementById;
                                            setSuggestionDismissed(true);
                                            setRepoPath("");
                                            (_document_getElementById = document.getElementById("backlog-repo-path")) === null || _document_getElementById === void 0 ? void 0 : _document_getElementById.focus();
                                        },
                                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            "aria-hidden": "true",
                                            children: "✕"
                                        })
                                    })
                                ]
                            }),
                            errors.repoPath && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                id: "backlog-repo-path-error",
                                className: BacklogItemForm_css/* errorMessage */.gJ,
                                role: "alert",
                                children: errors.repoPath
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: BacklogItemForm_css/* fieldGroup */.Y8,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                htmlFor: "backlog-priority",
                                className: BacklogItemForm_css/* label */.Pf,
                                children: "Priority"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("select", {
                                id: "backlog-priority",
                                className: BacklogItemForm_css/* select */.Lt,
                                value: priority,
                                onChange: (e)=>setPriority(Number(e.target.value)),
                                disabled: busy,
                                "data-testid": "backlog-priority-select",
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                        value: 1,
                                        children: "P1 — Critical"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                        value: 2,
                                        children: "P2 — High"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                        value: 3,
                                        children: "P3 — Medium"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                        value: 4,
                                        children: "P4 — Low"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                        value: 5,
                                        children: "P5 — Trivial"
                                    })
                                ]
                            })
                        ]
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: BacklogItemForm_css/* fieldGroup */.Y8,
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)(RadioGroup/* RadioGroup */.z, {
                    options: categoryOptions,
                    value: category,
                    onChange: handleCategoryChange,
                    groupLabel: "Category",
                    hintForValue: categoryHintForValue
                })
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: BacklogItemForm_css/* fieldGroup */.Y8,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)(RadioGroup/* RadioGroup */.z, {
                        options: pipelineModeOptions,
                        value: pipelineMode,
                        onChange: handlePipelineModeChange,
                        groupLabel: "Pipeline mode",
                        hintForValue: pipelineModeHintForValue,
                        trailingContent: unresolvedPipelineModeOption
                    }),
                    modesError && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        role: "status",
                        className: BacklogItemForm_css/* checkboxHint */.X7,
                        "data-testid": "backlog-pipeline-mode-fetch-error",
                        children: PIPELINE_MODE_FETCH_ERROR_NOTICE
                    }),
                    hasNoAvailableModes && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                        className: BacklogItemForm_css/* pipelineModeEmptyHint */.Iv,
                        "data-testid": "backlog-pipeline-mode-empty-hint",
                        children: [
                            "No custom pipeline modes exist yet.",
                            " ",
                            /*#__PURE__*/ (0,jsx_runtime.jsx)(nav/* Link */.N_, {
                                href: routes/* routes */.J.settingsPipelineModes,
                                className: BacklogItemForm_css/* pipelineModeEmptyHintLink */.CM,
                                children: "Create one in Settings →"
                            })
                        ]
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: BacklogItemForm_css/* fieldGroup */.Y8,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                        htmlFor: "backlog-cost-budget-threshold",
                        className: BacklogItemForm_css/* label */.Pf,
                        children: "Budget threshold (USD)"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                        id: "backlog-cost-budget-threshold",
                        type: "number",
                        min: "0",
                        step: "0.01",
                        className: BacklogItemForm_css/* input */.hF,
                        value: costBudgetThreshold,
                        onChange: (e)=>{
                            costBudgetThresholdTouchedRef.current = true;
                            setCostBudgetThreshold(e.target.value);
                        },
                        placeholder: "No threshold configured",
                        "aria-label": "Budget threshold in USD for this item",
                        "aria-invalid": !!errors.costBudgetThreshold,
                        "aria-describedby": errors.costBudgetThreshold ? "backlog-cost-budget-threshold-error" : undefined,
                        disabled: busy,
                        "data-testid": "backlog-cost-budget-threshold-input"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: BacklogItemForm_css/* checkboxHint */.X7,
                        children: "Show a warning banner on this item once its total cost reaches this amount. Leave blank for no budget tracking."
                    }),
                    errors.costBudgetThreshold && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        id: "backlog-cost-budget-threshold-error",
                        className: BacklogItemForm_css/* errorMessage */.gJ,
                        role: "alert",
                        children: errors.costBudgetThreshold
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("fieldset", {
                className: BacklogItemForm_css/* fieldGroup */.Y8,
                "data-testid": "backlog-overrides-fieldset",
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("legend", {
                        className: BacklogItemForm_css/* label */.Pf,
                        children: "Overrides (independent of pipeline mode)"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: BacklogItemForm_css/* twoColumn */.bu,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: BacklogItemForm_css/* fieldGroup */.Y8,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                        className: BacklogItemForm_css/* checkboxRow */.yc,
                                        htmlFor: "backlog-skip-planning",
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                id: "backlog-skip-planning",
                                                type: "checkbox",
                                                className: BacklogItemForm_css/* checkboxInput */.m8,
                                                checked: skipPlanning,
                                                onChange: (e)=>setSkipPlanning(e.target.checked),
                                                disabled: busy,
                                                "data-testid": "backlog-skip-planning-checkbox"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: BacklogItemForm_css/* checkboxLabel */.mH,
                                                children: "Skip planning phase"
                                            })
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: BacklogItemForm_css/* checkboxHint */.X7,
                                        children: "Go straight to triage without a separate planning pass."
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: BacklogItemForm_css/* fieldGroup */.Y8,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                        className: BacklogItemForm_css/* checkboxRow */.yc,
                                        htmlFor: "backlog-skip-review",
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                id: "backlog-skip-review",
                                                type: "checkbox",
                                                className: BacklogItemForm_css/* checkboxInput */.m8,
                                                checked: skipReviewGate,
                                                onChange: (e)=>setSkipReviewGate(e.target.checked),
                                                disabled: busy,
                                                "data-testid": "backlog-skip-review-checkbox"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: BacklogItemForm_css/* checkboxLabel */.mH,
                                                children: "Skip review gate"
                                            })
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: BacklogItemForm_css/* checkboxHint */.X7,
                                        children: "Mark work done without an automated review pass first."
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: BacklogItemForm_css/* fieldGroup */.Y8,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                        className: BacklogItemForm_css/* checkboxRow */.yc,
                                        htmlFor: "backlog-auto-spawn-session",
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                id: "backlog-auto-spawn-session",
                                                type: "checkbox",
                                                className: BacklogItemForm_css/* checkboxInput */.m8,
                                                checked: autoSpawnSession,
                                                onChange: (e)=>setAutoSpawnSession(e.target.checked),
                                                disabled: busy,
                                                "data-testid": "backlog-auto-spawn-session-checkbox"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: BacklogItemForm_css/* checkboxLabel */.mH,
                                                children: "Auto-spawn work session"
                                            })
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: BacklogItemForm_css/* checkboxHint */.X7,
                                        children: 'Skip the manual "Spawn Session" click — start work automatically once triage marks the item ready.'
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: BacklogItemForm_css/* fieldGroup */.Y8,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                        className: BacklogItemForm_css/* checkboxRow */.yc,
                                        htmlFor: "backlog-auto-create-pr",
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                id: "backlog-auto-create-pr",
                                                type: "checkbox",
                                                className: BacklogItemForm_css/* checkboxInput */.m8,
                                                checked: autoCreatePR,
                                                onChange: (e)=>setAutoCreatePR(e.target.checked),
                                                disabled: busy,
                                                "data-testid": "backlog-auto-create-pr-checkbox"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: BacklogItemForm_css/* checkboxLabel */.mH,
                                                children: "Auto-create PR on completion"
                                            })
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: BacklogItemForm_css/* checkboxHint */.X7,
                                        children: 'Skip the manual Review Queue "Create PR" click — a PR is opened automatically once a work session finishes. The prompt still runs unattended, so review the diff before merging.'
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: BacklogItemForm_css/* fieldGroup */.Y8,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                        className: BacklogItemForm_css/* checkboxRow */.yc,
                                        htmlFor: "backlog-auto-approve-plan",
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                id: "backlog-auto-approve-plan",
                                                type: "checkbox",
                                                className: BacklogItemForm_css/* checkboxInput */.m8,
                                                checked: autoApprovePlan,
                                                onChange: (e)=>setAutoApprovePlan(e.target.checked),
                                                disabled: busy,
                                                "data-testid": "backlog-auto-approve-plan-checkbox"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: BacklogItemForm_css/* checkboxLabel */.mH,
                                                children: "Auto-approve plan"
                                            })
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: BacklogItemForm_css/* checkboxHint */.X7,
                                        children: 'Skip the manual "Approve Plan" click — a plan produced by triage is approved automatically and moves straight to implementation. Review the plan afterward instead of before.'
                                    })
                                ]
                            })
                        ]
                    })
                ]
            }),
            showRepoPathPrerequisiteWarning && selectedPipelineMode && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                role: "alert",
                className: BacklogItemForm_css/* errorMessage */.gJ,
                "data-testid": "backlog-pipeline-mode-repo-path-warning",
                children: "⚠ ".concat(selectedPipelineMode.name, " mode requires a repository path — add one above.")
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: BacklogItemForm_css/* acSection */.oL,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: BacklogItemForm_css/* acSectionHeader */.dG,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                className: BacklogItemForm_css/* label */.Pf,
                                children: "Acceptance Criteria"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: BacklogItemForm_css/* addButton */.c9,
                                onClick: addCriterion,
                                disabled: busy,
                                "aria-label": "Add acceptance criterion",
                                "data-testid": "backlog-add-criterion",
                                children: "+ Add criterion"
                            })
                        ]
                    }),
                    acCriteria.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: BacklogItemForm_css/* acList */.lG,
                        role: "list",
                        "aria-label": "Acceptance criteria list",
                        children: acCriteria.map((criterion, i)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: BacklogItemForm_css/* acRow */.bK,
                                role: "listitem",
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                        type: "text",
                                        className: BacklogItemForm_css/* acInput */.TQ,
                                        value: criterion.text,
                                        onChange: (e)=>updateCriterionText(i, e.target.value),
                                        placeholder: "Criterion ".concat(i + 1),
                                        disabled: busy,
                                        "aria-label": "Criterion ".concat(i + 1, " text"),
                                        "data-testid": "backlog-criterion-text-".concat(i)
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("select", {
                                        className: BacklogItemForm_css/* acStatusSelect */.dR,
                                        value: criterion.status,
                                        onChange: (e)=>updateCriterionStatus(i, e.target.value),
                                        disabled: busy,
                                        "aria-label": "Criterion ".concat(i + 1, " status"),
                                        "data-testid": "backlog-criterion-status-".concat(i),
                                        children: AC_STATUS_OPTIONS.map((opt)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                                value: opt.value,
                                                children: opt.label
                                            }, opt.value))
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        type: "button",
                                        className: BacklogItemForm_css/* removeButton */.DT,
                                        onClick: ()=>removeCriterion(i),
                                        disabled: busy,
                                        "aria-label": "Remove criterion ".concat(i + 1),
                                        "data-testid": "backlog-remove-criterion-".concat(i),
                                        children: "\xd7"
                                    })
                                ]
                            }, i))
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: BacklogItemForm_css/* formActions */.AU,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: BacklogItemForm_css/* cancelButton */.x7,
                        onClick: onCancel,
                        disabled: busy,
                        "data-testid": "backlog-form-cancel",
                        children: "Cancel"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "submit",
                        className: BacklogItemForm_css/* submitButton */.RC,
                        disabled: busy,
                        "data-testid": "backlog-form-submit",
                        children: busy ? isCloningRepo ? "Cloning repository…" : "Saving…" : (initialValues === null || initialValues === void 0 ? void 0 : initialValues.id) ? "Save Changes" : "Create Item"
                    })
                ]
            })
        ]
    });
}


/***/ })

}]);
//# sourceMappingURL=1618-5e39ec59d53e7833.js.map