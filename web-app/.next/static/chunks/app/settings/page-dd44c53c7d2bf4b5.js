(self["webpackChunk_N_E"] = self["webpackChunk_N_E"] || []).push([[4662],{

/***/ 3335:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   CC: () => (/* binding */ formatTimestampShort),
/* harmony export */   Ey: () => (/* binding */ formatTimestamp),
/* harmony export */   dB: () => (/* binding */ getUserTimezone),
/* harmony export */   fw: () => (/* binding */ formatRelativeTime),
/* harmony export */   jX: () => (/* binding */ TIME_RANGE_PRESETS)
/* harmony export */ });
/* unused harmony export formatTime */
/**
 * Utility functions for date and time formatting with timezone support
 */ /**
 * Format a timestamp for display with full date, time, and timezone
 * @param date - Date object or timestamp to format
 * @returns Formatted string in user's local timezone (e.g., "1/5/2025, 3:45:30 PM PST")
 */ function formatTimestamp(date) {
    const dateObj = typeof date === 'number' ? new Date(date) : date;
    return dateObj.toLocaleString(undefined, {
        year: 'numeric',
        month: 'numeric',
        day: 'numeric',
        hour: 'numeric',
        minute: '2-digit',
        second: '2-digit',
        timeZoneName: 'short'
    });
}
/**
 * Format a timestamp for display with date and time (no timezone)
 * @param date - Date object or timestamp to format
 * @returns Formatted string in user's local timezone (e.g., "1/5/2025, 3:45:30 PM")
 */ function formatTimestampShort(date) {
    const dateObj = typeof date === 'number' ? new Date(date) : date;
    return dateObj.toLocaleString(undefined, {
        year: 'numeric',
        month: 'numeric',
        day: 'numeric',
        hour: 'numeric',
        minute: '2-digit',
        second: '2-digit'
    });
}
/**
 * Format a timestamp for display with just time (no date)
 * @param date - Date object or timestamp to format
 * @returns Formatted string in user's local timezone (e.g., "3:45:30 PM")
 */ function formatTime(date) {
    const dateObj = typeof date === 'number' ? new Date(date) : date;
    return dateObj.toLocaleTimeString(undefined, {
        hour: 'numeric',
        minute: '2-digit',
        second: '2-digit'
    });
}
/**
 * Format a timestamp as relative time (e.g., "5m ago", "2h ago")
 * @param timestamp - Unix timestamp in milliseconds
 * @returns Relative time string
 */ function formatRelativeTime(timestamp) {
    const date = new Date(timestamp);
    const now = new Date();
    const diffMs = now.getTime() - date.getTime();
    const diffMins = Math.floor(diffMs / 60000);
    const diffHours = Math.floor(diffMs / 3600000);
    const diffDays = Math.floor(diffMs / 86400000);
    if (diffMins < 1) return "Just now";
    if (diffMins < 60) return "".concat(diffMins, "m ago");
    if (diffHours < 24) return "".concat(diffHours, "h ago");
    if (diffDays < 7) return "".concat(diffDays, "d ago");
    return date.toLocaleDateString();
}
/**
 * Get the user's timezone abbreviation (e.g., "PST", "EST", "UTC")
 */ function getUserTimezone() {
    const date = new Date();
    const timeZoneName = date.toLocaleString(undefined, {
        timeZoneName: 'short'
    }).split(' ').pop();
    return timeZoneName || Intl.DateTimeFormat().resolvedOptions().timeZone;
}
/**
 * Standard time range presets for log filtering
 */ const TIME_RANGE_PRESETS = [
    {
        label: 'Last 5 minutes',
        value: '5m',
        getRange: ()=>({
                start: new Date(Date.now() - 5 * 60 * 1000),
                end: new Date()
            })
    },
    {
        label: 'Last 15 minutes',
        value: '15m',
        getRange: ()=>({
                start: new Date(Date.now() - 15 * 60 * 1000),
                end: new Date()
            })
    },
    {
        label: 'Last 1 hour',
        value: '1h',
        getRange: ()=>({
                start: new Date(Date.now() - 60 * 60 * 1000),
                end: new Date()
            })
    },
    {
        label: 'Last 4 hours',
        value: '4h',
        getRange: ()=>({
                start: new Date(Date.now() - 4 * 60 * 60 * 1000),
                end: new Date()
            })
    },
    {
        label: 'Last 24 hours',
        value: '24h',
        getRange: ()=>({
                start: new Date(Date.now() - 24 * 60 * 60 * 1000),
                end: new Date()
            })
    },
    {
        label: 'Last 7 days',
        value: '7d',
        getRange: ()=>({
                start: new Date(Date.now() - 7 * 24 * 60 * 60 * 1000),
                end: new Date()
            })
    },
    {
        label: 'All time',
        value: 'all',
        getRange: ()=>({
                start: new Date(0),
                end: new Date()
            })
    }
];


/***/ }),

/***/ 27043:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   t: () => (/* binding */ useInputModeOverride)
/* harmony export */ });
/* unused harmony export INPUT_MODE_OVERRIDE_KEY */
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(55494);
/* __next_internal_client_entry_do_not_use__ INPUT_MODE_OVERRIDE_KEY,useInputModeOverride auto */ 
const INPUT_MODE_OVERRIDE_KEY = "stapler-squad:input-mode-override";
/**
 * Reads and persists the user's override for mouse+keyboard-on-mobile detection.
 *
 * "auto" (default) lets the terminal toolbar decide based on live hardware
 * signals (matchMedia hover/pointer:fine via useViewport().hasFinePointer).
 * "desktop" and "touch" force that decision regardless of what the hardware
 * reports, for the rare case detection gets it wrong (e.g. some Android
 * keyboard cases, or a user who prefers the expanded toolbar despite a mouse).
 */ function useInputModeOverride() {
    // Read synchronously via a lazy initializer (not a mount effect) — a caller that
    // derives a one-time default from inputModeOverride on its own mount effect would
    // otherwise see the "auto" fallback before an async localStorage-read effect here
    // resolved the persisted value, and could apply the wrong default irreversibly.
    const [inputModeOverride, setInputModeOverrideState] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(()=>{
        if (typeof localStorage === "undefined") return "auto";
        try {
            const stored = localStorage.getItem(INPUT_MODE_OVERRIDE_KEY);
            if (stored === "auto" || stored === "desktop" || stored === "touch") {
                return stored;
            }
        } catch (e) {}
        return "auto";
    });
    const setInputModeOverride = (next)=>{
        setInputModeOverrideState(next);
        try {
            localStorage.setItem(INPUT_MODE_OVERRIDE_KEY, next);
        } catch (e) {}
    };
    return {
        inputModeOverride,
        setInputModeOverride
    };
}


/***/ }),

/***/ 31813:
/***/ ((__unused_webpack_module, __unused_webpack_exports, __webpack_require__) => {

Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 90433));


/***/ }),

/***/ 81073:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Ai: () => (/* binding */ DEFAULT_TERMINAL_CONFIG),
/* harmony export */   Ez: () => (/* binding */ lightTerminalTheme),
/* harmony export */   Gd: () => (/* binding */ darkTerminalTheme),
/* harmony export */   X6: () => (/* binding */ loadTerminalConfig),
/* harmony export */   wo: () => (/* binding */ saveTerminalConfig)
/* harmony export */ });
/* unused harmony export TERMINAL_CONFIG_PRESETS */
/**
 * Terminal configuration system with localStorage persistence
 *
 * Provides user-configurable settings for terminal behavior, appearance,
 * and performance. All settings persist across sessions.
 */ /**
 * Default terminal configuration
 * These values provide good performance and UX for most users
 */ const DEFAULT_TERMINAL_CONFIG = {
    scrollbackLines: 0,
    theme: "dark",
    fontSize: 14,
    fontFamily: 'Menlo, Monaco, "Courier New", monospace',
    enableBell: false,
    cursorStyle: "block",
    cursorBlink: true,
    mouseTracking: "none"
};
/**
 * Configuration presets for different use cases
 */ const TERMINAL_CONFIG_PRESETS = {
    "default": DEFAULT_TERMINAL_CONFIG,
    "high-performance": {
        scrollbackLines: 0,
        cursorBlink: false
    },
    "accessibility": {
        fontSize: 16,
        cursorStyle: "block",
        cursorBlink: false,
        theme: "light"
    }
};
/**
 * Named dark terminal theme for xterm.js
 */ const darkTerminalTheme = {
    background: '#1e1e1e',
    foreground: '#cccccc',
    cursor: '#cccccc',
    cursorAccent: '#1e1e1e',
    selectionBackground: 'rgba(255, 255, 255, 0.3)',
    black: '#000000',
    red: '#cd3131',
    green: '#0dbc79',
    yellow: '#e5e510',
    blue: '#2472c8',
    magenta: '#bc3fbc',
    cyan: '#11a8cd',
    white: '#e5e5e5',
    brightBlack: '#666666',
    brightRed: '#f14c4c',
    brightGreen: '#23d18b',
    brightYellow: '#f5f543',
    brightBlue: '#3b8eea',
    brightMagenta: '#d670d6',
    brightCyan: '#29b8db',
    brightWhite: '#ffffff'
};
/**
 * Named light terminal theme for xterm.js
 */ const lightTerminalTheme = {
    background: '#ffffff',
    foreground: '#1a1a1a',
    cursor: '#333333',
    selectionBackground: 'rgba(0, 112, 243, 0.3)',
    black: '#000000',
    red: '#c0392b',
    green: '#27ae60',
    yellow: '#e67e22',
    blue: '#2980b9',
    magenta: '#8e44ad',
    cyan: '#16a085',
    white: '#bdc3c7',
    brightBlack: '#7f8c8d',
    brightRed: '#e74c3c',
    brightGreen: '#2ecc71',
    brightYellow: '#f39c12',
    brightBlue: '#3498db',
    brightMagenta: '#9b59b6',
    brightCyan: '#1abc9c',
    brightWhite: '#ecf0f1'
};
const STORAGE_KEY = "stapler-squad-terminal-config";
/**
 * Load terminal configuration from localStorage
 * Falls back to default config if not found or invalid
 */ function loadTerminalConfig() {
    if (false) {}
    try {
        const stored = localStorage.getItem(STORAGE_KEY);
        if (!stored) {
            return DEFAULT_TERMINAL_CONFIG;
        }
        const config = JSON.parse(stored);
        var _config_scrollbackLines, _config_fontSize;
        // Validate and merge with defaults
        return {
            ...DEFAULT_TERMINAL_CONFIG,
            ...config,
            // Ensure scrollbackLines is within reasonable bounds (0 = disabled, tmux handles it)
            scrollbackLines: Math.max(0, Math.min((_config_scrollbackLines = config.scrollbackLines) !== null && _config_scrollbackLines !== void 0 ? _config_scrollbackLines : DEFAULT_TERMINAL_CONFIG.scrollbackLines, 100000)),
            // Ensure fontSize is within reasonable bounds
            fontSize: Math.max(8, Math.min((_config_fontSize = config.fontSize) !== null && _config_fontSize !== void 0 ? _config_fontSize : DEFAULT_TERMINAL_CONFIG.fontSize, 32))
        };
    } catch (err) {
        console.error("[terminalConfig] Failed to load config:", err);
        return DEFAULT_TERMINAL_CONFIG;
    }
}
/**
 * Save terminal configuration to localStorage
 */ function saveTerminalConfig(config) {
    if (false) {}
    try {
        const current = loadTerminalConfig();
        const updated = {
            ...current,
            ...config
        };
        localStorage.setItem(STORAGE_KEY, JSON.stringify(updated));
        // Dispatch custom event for other components to react
        window.dispatchEvent(new CustomEvent("terminal-config-changed", {
            detail: updated
        }));
    } catch (err) {
        console.error("[terminalConfig] Failed to save config:", err);
    }
}


/***/ }),

/***/ 90433:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
// ESM COMPAT FLAG
__webpack_require__.r(__webpack_exports__);

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  "default": () => (/* binding */ SettingsPage)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/index.js
var react = __webpack_require__(55494);
// EXTERNAL MODULE: ./src/lib/nav/index.ts
var nav = __webpack_require__(41078);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@radix-ui+react-tabs@1.1.13+b721ee6ccd61acaa/node_modules/@radix-ui/react-tabs/dist/index.mjs + 3 modules
var dist = __webpack_require__(51586);
// EXTERNAL MODULE: ./src/gen/session/v1/session_pb.ts
var session_pb = __webpack_require__(23575);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/promise-client.js + 2 modules
var promise_client = __webpack_require__(66144);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect-web@2.1.1+0149f91553ca585e/node_modules/@connectrpc/connect-web/dist/esm/connect-transport.js + 8 modules
var connect_transport = __webpack_require__(46564);
// EXTERNAL MODULE: ./src/lib/config.ts
var config = __webpack_require__(51202);
// EXTERNAL MODULE: ./src/lib/constants/programs.ts
var programs = __webpack_require__(49051);
// EXTERNAL MODULE: ./src/components/ui/RepoPathInput.tsx + 2 modules
var RepoPathInput = __webpack_require__(34609);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsettings%2FGlobalDefaultsForm.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA9VVy26DMBC85yv22B6MbNrm4XxMZWyHWAUb2SZpU_XfK5wEHAIojXLpBYRZZndnZpfEOSY4x_A9AxDKVQX7orAp5Od6BuGOhLKSe2U0BW6KutTNm5xVFIiV5Xr2M0uOICSAcFMYS2HH7BNC72QlD6k94OcAZ7RHTh0kBZKkb-Hr0-leqnzrKcwxbs5KZnOlKeAIPh2HT5-juJfpuIsycLIkbSFDSV_vI-bcXYvzdhcOTnow8_HeyEBvi3GOW8hFgKyYEErn55SAk_bbjPGP3JpaCzScmIXMmbFCWgqk-gRnCiV6YR9RFLJMqNpReK0CDcO42XhHXfV0Y3jtQg-m9oXSkoI2WkbJBtGL2DLL_8jB0T0USNzIQ-hYTZh1bxtjNtfIpS99WdglgtJNJegMxAqVa6S8LB0FLrWXNgJr5zHSIz0JcrMei5v02AzpQdIJQa4WWbRB2u6z0H1XY1-C7nliS_HauuZdZdSZoIu0i2uaACfdNguMb08jH3sko1uzk3Zik-DYDHzYDBf7aUTSFkM8ZvvJB5Sy6Y_60bt_GnZyk7kI_vO0T_-fBhyx3yovkasYD3N-HMuu10hpUzGu_FcDuYpC8psoPdGFvIll-QUh_T3fPAgAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsettings_2FGlobalDefaultsForm_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA9VVy26DMBC85yv22B6MbNrm4XxMZWyHWAUb2SZpU_XfK5wEHAIojXLpBYRZZndnZpfEOSY4x_A9AxDKVQX7orAp5Od6BuGOhLKSe2U0BW6KutTNm5xVFIiV5Xr2M0uOICSAcFMYS2HH7BNC72QlD6k94OcAZ7RHTh0kBZKkb_Hr0_leqnzrKcwxbs5KZnOlKeAIPh2HT5_juJfpuIsycLIkbSFDSV_vI_bcXYvzdhcOTnow8_HeyEBvi3GOW8hFgKyYEErn55SAk_bbjPGP3JpaCzScmIXMmbFCWgqk_gRnCiV6YR9RFLJMqNpReK0CDcO42XhHXfV0Y3jtQg_m9oXSkoI2WkbJBtGL2DLL_8jB0T0USNzIQ_hYTZh1bxtjNtfIpS99WdglgtJNJegMxAqVa6S8LB0FLrWXNgJr5zHSIz0JcrMei5v02AzpQdIJQa4WWbRB2u6z0H1XY1_C7nliS_HauuZdZdSZoIu0i2uaACfdNguMb08jH3sko1uzk3Zik_DYDHzYDBf7aUTSFkM8ZvvJB5Sy6Y_60bt_GnZyk7kI_vO0T_fBhyx3yovkasYD3N_HMuu10hpUzGu_FcDuYpC8psoPdGFvIll_QUh_T3fPAgAAA_22_7D = __webpack_require__(10900);
;// ./src/components/settings/GlobalDefaultsForm.css.ts


var actions = 'ssadccg';
var container = 'ssadcc0';
var deleteBtn = 'ssadccf';
var envVarRow = 'ssadcce';
var envVarTable = 'ssadccd';
var field = 'ssadcc5';
var GlobalDefaultsForm_css_form = 'ssadcc4';
var heading = 'ssadcc1';
var hint = 'ssadcc3';
var input = 'ssadcc8';
var label = 'ssadcc6';
var loadingText = 'ssadcc2';
var GlobalDefaultsForm_css_select = 'ssadcc7';
var tag = 'ssadcca';
var tagInputRow = 'ssadccc';
var tagList = 'ssadcc9';
var tagRemove = 'ssadccb';
;// ./src/components/settings/GlobalDefaultsForm.tsx
/* __next_internal_client_entry_do_not_use__ GlobalDefaultsForm auto */ 








// Mirrors config.AutonomousMaxTurnsOrDefault's [1, 200] bound; the server re-clamps regardless.
function clampAutonomousMaxTurns(raw) {
    return Math.min(200, Math.max(1, parseInt(raw, 10) || 1));
}
function GlobalDefaultsForm() {
    const [program, setProgram] = (0,react.useState)("");
    const [oneOffBaseDir, setOneOffBaseDir] = (0,react.useState)("");
    const [newProjectBaseDir, setNewProjectBaseDir] = (0,react.useState)("");
    const [tags, setTags] = (0,react.useState)([]);
    const [tagInput, setTagInput] = (0,react.useState)("");
    const [envVars, setEnvVars] = (0,react.useState)([]);
    const [cliFlags, setCliFlags] = (0,react.useState)("");
    const [maxAutoReworkIterations, setMaxAutoReworkIterations] = (0,react.useState)(0); // set from the server-resolved default on load
    // String draft so clearing/retyping the field isn't rewritten mid-edit; clamped on blur and save.
    const [autonomousMaxTurns, setAutonomousMaxTurns] = (0,react.useState)(""); // set from the server-resolved default on load
    const [maxConcurrentBacklogWorkItems, setMaxConcurrentBacklogWorkItems] = (0,react.useState)(2);
    const [staleSessionThresholdMinutes, setStaleSessionThresholdMinutes] = (0,react.useState)(30);
    const [staleSessionNotifyEnabled, setStaleSessionNotifyEnabled] = (0,react.useState)(true);
    const [loading, setLoading] = (0,react.useState)(true);
    const [saving, setSaving] = (0,react.useState)(false);
    const [error, setError] = (0,react.useState)(null);
    const [success, setSuccess] = (0,react.useState)(null);
    const clientRef = (0,react.useRef)(null);
    const loadDefaults = (0,react.useCallback)(async ()=>{
        if (!clientRef.current) return;
        try {
            setLoading(true);
            setError(null);
            const response = await clientRef.current.getSessionDefaults({});
            const defaults = response.defaults;
            if (defaults) {
                setProgram(defaults.program);
                setOneOffBaseDir(defaults.oneOffBaseDir);
                setNewProjectBaseDir(defaults.newProjectBaseDir);
                setTags([
                    ...defaults.tags
                ]);
                setCliFlags(defaults.cliFlags);
                setMaxAutoReworkIterations(defaults.maxAutoReworkIterations);
                setAutonomousMaxTurns(String(defaults.autonomousMaxTurns));
                setMaxConcurrentBacklogWorkItems(defaults.maxConcurrentBacklogWorkItems || 2);
                setStaleSessionThresholdMinutes(defaults.staleSessionThresholdMinutes || 30);
                setStaleSessionNotifyEnabled(defaults.staleSessionNotifyEnabled);
                const vars = Object.entries(defaults.envVars).map((param)=>{
                    let [key, value] = param;
                    return {
                        key,
                        value
                    };
                });
                setEnvVars(vars);
            }
        } catch (err) {
            setError("Failed to load defaults: ".concat(err));
        } finally{
            setLoading(false);
        }
    }, []);
    (0,react.useEffect)(()=>{
        const transport = (0,connect_transport/* createConnectTransport */.w)({
            baseUrl: (0,config/* getApiBaseUrl */.h)()
        });
        clientRef.current = (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, transport);
        loadDefaults();
    }, [
        loadDefaults
    ]);
    const handleSave = async ()=>{
        if (!clientRef.current) return;
        try {
            setSaving(true);
            setError(null);
            setSuccess(null);
            const envVarsMap = {};
            for (const { key, value } of envVars){
                if (key.trim()) {
                    envVarsMap[key.trim()] = value;
                }
            }
            await clientRef.current.updateGlobalDefaults({
                program,
                oneOffBaseDir,
                newProjectBaseDir,
                tags,
                envVars: envVarsMap,
                cliFlags,
                maxAutoReworkIterations,
                autonomousMaxTurns: clampAutonomousMaxTurns(autonomousMaxTurns),
                maxConcurrentBacklogWorkItems,
                staleSessionThresholdMinutes,
                staleSessionNotifyEnabled
            });
            setSuccess("Global defaults saved.");
            setTimeout(()=>setSuccess(null), 3000);
        } catch (err) {
            setError("Failed to save defaults: ".concat(err));
        } finally{
            setSaving(false);
        }
    };
    const handleAddTag = ()=>{
        const trimmed = tagInput.trim();
        if (trimmed && !tags.includes(trimmed)) {
            setTags([
                ...tags,
                trimmed
            ]);
        }
        setTagInput("");
    };
    const handleRemoveTag = (tag)=>{
        setTags(tags.filter((t)=>t !== tag));
    };
    const handleTagKeyDown = (e)=>{
        if (e.key === "Enter") {
            e.preventDefault();
            handleAddTag();
        }
    };
    const handleAddEnvVar = ()=>{
        setEnvVars([
            ...envVars,
            {
                key: "",
                value: ""
            }
        ]);
    };
    const handleRemoveEnvVar = (index)=>{
        setEnvVars(envVars.filter((_, i)=>i !== index));
    };
    const handleEnvVarChange = (index, field, val)=>{
        const updated = [
            ...envVars
        ];
        updated[index] = {
            ...updated[index],
            [field]: val
        };
        setEnvVars(updated);
    };
    if (loading) {
        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            className: container,
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                    className: heading,
                    children: "Global Defaults"
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: loadingText,
                    children: "Loading..."
                })
            ]
        });
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: container,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                className: heading,
                children: "Global Defaults"
            }),
            error && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: "alert alert-error",
                children: error
            }),
            success && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: "alert alert-success",
                children: success
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: GlobalDefaultsForm_css_form,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: field,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                className: label,
                                htmlFor: "global-program",
                                children: "Program"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("select", {
                                id: "global-program",
                                className: GlobalDefaultsForm_css_select,
                                value: program,
                                onChange: (e)=>setProgram(e.target.value),
                                children: programs/* PROGRAMS */.i.map((p)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                        value: p.value,
                                        children: p.label
                                    }, p.value))
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: field,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                className: label,
                                htmlFor: "global-one-off-base-dir",
                                children: "One-off Session Directory"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)(RepoPathInput/* RepoPathInput */.E, {
                                id: "global-one-off-base-dir",
                                placeholder: "~/oneoff",
                                value: oneOffBaseDir,
                                onChange: setOneOffBaseDir
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: field,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                className: label,
                                htmlFor: "global-new-project-base-dir",
                                children: "New Project Base Directory"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)(RepoPathInput/* RepoPathInput */.E, {
                                id: "global-new-project-base-dir",
                                placeholder: "~/Projects",
                                value: newProjectBaseDir,
                                onChange: setNewProjectBaseDir
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: field,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                className: label,
                                children: "Tags"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: tagList,
                                children: tags.map((t)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                        className: tag,
                                        children: [
                                            t,
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                type: "button",
                                                className: tagRemove,
                                                onClick: ()=>handleRemoveTag(t),
                                                "aria-label": "Remove tag ".concat(t),
                                                children: "x"
                                            })
                                        ]
                                    }, t))
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: tagInputRow,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                        type: "text",
                                        className: input,
                                        placeholder: "Add a tag...",
                                        value: tagInput,
                                        onChange: (e)=>setTagInput(e.target.value),
                                        onKeyDown: handleTagKeyDown
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        type: "button",
                                        className: "btn btn-secondary",
                                        onClick: handleAddTag,
                                        children: "Add"
                                    })
                                ]
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: field,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                className: label,
                                children: "Environment Variables"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: envVarTable,
                                children: envVars.map((envVar, i)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        className: envVarRow,
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                type: "text",
                                                className: input,
                                                placeholder: "KEY",
                                                value: envVar.key,
                                                onChange: (e)=>handleEnvVarChange(i, "key", e.target.value)
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                type: "text",
                                                className: input,
                                                placeholder: "value",
                                                value: envVar.value,
                                                onChange: (e)=>handleEnvVarChange(i, "value", e.target.value)
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                type: "button",
                                                className: deleteBtn,
                                                onClick: ()=>handleRemoveEnvVar(i),
                                                "aria-label": "Remove environment variable",
                                                children: "Delete"
                                            })
                                        ]
                                    }, i))
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: "btn btn-secondary",
                                onClick: handleAddEnvVar,
                                children: "Add Variable"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: field,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                className: label,
                                htmlFor: "global-cli-flags",
                                children: "CLI Flags"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                id: "global-cli-flags",
                                type: "text",
                                className: input,
                                placeholder: "e.g. --verbose --no-color",
                                value: cliFlags,
                                onChange: (e)=>setCliFlags(e.target.value)
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: field,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                className: label,
                                htmlFor: "global-max-auto-rework-iterations",
                                children: "Max Auto-Rework Iterations"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                id: "global-max-auto-rework-iterations",
                                type: "number",
                                min: 1,
                                className: input,
                                value: maxAutoReworkIterations,
                                onChange: (e)=>setMaxAutoReworkIterations(Math.max(1, Number(e.target.value) || 1))
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                className: hint,
                                children: "How many times a backlog item can be auto-reopened for rework after a failed review before it's left in review for manual action."
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: field,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                className: label,
                                htmlFor: "global-autonomous-max-turns",
                                children: "Max Autonomous Session Turns"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                id: "global-autonomous-max-turns",
                                type: "number",
                                min: 1,
                                max: 200,
                                className: input,
                                inputMode: "numeric",
                                "aria-describedby": "global-autonomous-max-turns-hint",
                                value: autonomousMaxTurns,
                                onChange: (e)=>setAutonomousMaxTurns(e.target.value),
                                onBlur: ()=>setAutonomousMaxTurns(String(clampAutonomousMaxTurns(autonomousMaxTurns)))
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                id: "global-autonomous-max-turns-hint",
                                className: hint,
                                children: "How many turns a single autonomous session may run before it stops. Default 30, maximum 200. Too low cuts work off mid-task; too high risks runaway cost."
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: field,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                className: label,
                                htmlFor: "global-max-concurrent-backlog-work-items",
                                children: "Max Concurrent Backlog Work Items"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                id: "global-max-concurrent-backlog-work-items",
                                type: "number",
                                min: 1,
                                max: 10,
                                className: input,
                                value: maxConcurrentBacklogWorkItems,
                                onChange: (e)=>setMaxConcurrentBacklogWorkItems(Math.min(10, Math.max(1, Number(e.target.value) || 1)))
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                className: hint,
                                children: "How many backlog items can have a live work session at once. Items spawned beyond this cap are queued and start automatically as slots free up."
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: field,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                className: label,
                                htmlFor: "global-stale-session-threshold-minutes",
                                children: "Stale Session Threshold (minutes)"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                id: "global-stale-session-threshold-minutes",
                                "data-testid": "stale-session-threshold-input",
                                type: "number",
                                min: 1,
                                className: input,
                                value: staleSessionThresholdMinutes,
                                onChange: (e)=>setStaleSessionThresholdMinutes(Math.max(1, Number(e.target.value) || 1))
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                className: hint,
                                children: "How long a session can sit idle with no activity before it's flagged as stale."
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: field,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                className: label,
                                htmlFor: "global-stale-session-notify-enabled",
                                children: "Notify on Stale Sessions"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                id: "global-stale-session-notify-enabled",
                                "data-testid": "stale-session-notify-checkbox",
                                type: "checkbox",
                                checked: staleSessionNotifyEnabled,
                                onChange: (e)=>setStaleSessionNotifyEnabled(e.target.checked)
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                className: hint,
                                children: "Send a notification when a session is detected as stale."
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: actions,
                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            type: "button",
                            className: "btn btn-primary",
                            onClick: handleSave,
                            disabled: saving,
                            children: saving ? "Saving..." : "Save"
                        })
                    })
                ]
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2FformCard.css.ts.vanilla.css%22%2C%22source%22%3A%22Ll8xOXh2YmVsMCB7CiAgZGlzcGxheTogZmxleDsKICBnYXA6IDAuMzc1cmVtOwogIGZsZXgtc2hyaW5rOiAwOwp9Ci5fMTl4dmJlbDEgewogIHBhZGRpbmc6IDFyZW07CiAgYmFja2dyb3VuZC1jb2xvcjogdmFyKC0tXzE5ZXoycno3KTsKICBib3JkZXI6IDFweCBzb2xpZCB2YXIoLS1fMTllejJyemUpOwogIGJvcmRlci1yYWRpdXM6IDZweDsKfQouXzE5eHZiZWwyIHsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnowKTsKICBmb250LXNpemU6IDFyZW07CiAgZm9udC13ZWlnaHQ6IDYwMDsKICBtYXJnaW46IDAgMCAwLjc1cmVtIDA7Cn0KLl8xOXh2YmVsMyB7CiAgZGlzcGxheTogZmxleDsKICBmbGV4LWRpcmVjdGlvbjogY29sdW1uOwogIGdhcDogMC43NXJlbTsKfQouXzE5eHZiZWw0IHsKICBkaXNwbGF5OiBmbGV4OwogIGZsZXgtZGlyZWN0aW9uOiBjb2x1bW47CiAgZ2FwOiAwLjM3NXJlbTsKfQouXzE5eHZiZWw1IHsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoxKTsKICBmb250LXNpemU6IDAuODEyNXJlbTsKICBmb250LXdlaWdodDogNjAwOwp9Ci5fMTl4dmJlbDYgewogIGRpc3BsYXk6IGZsZXg7CiAgYWxpZ24taXRlbXM6IGNlbnRlcjsKICBnYXA6IDAuNXJlbTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoxKTsKICBmb250LXNpemU6IDAuODEyNXJlbTsKICBmb250LXdlaWdodDogNjAwOwogIGN1cnNvcjogcG9pbnRlcjsKfQouXzE5eHZiZWw3IHsKICBwYWRkaW5nOiAwLjVyZW0gMC43NXJlbTsKICBiYWNrZ3JvdW5kLWNvbG9yOiB2YXIoLS1fMTllejJyejFhKTsKICBib3JkZXI6IDFweCBzb2xpZCB2YXIoLS1fMTllejJyemspOwogIGJvcmRlci1yYWRpdXM6IDRweDsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoxYik7CiAgZm9udC1zaXplOiAwLjg3NXJlbTsKICBmbGV4OiAxOwp9Ci5fMTl4dmJlbDc6Zm9jdXMgewogIG91dGxpbmU6IG5vbmU7CiAgYm9yZGVyLWNvbG9yOiB2YXIoLS1fMTllejJyemwpOwp9%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2FformCard_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll8xOXh2YmVsMCB7CiAgZGlzcGxheTogZmxleDsKICBnYXA6IDAuMzc1cmVtOwogIGZsZXgtc2hyaW5rOiAwOwp9Ci5fMTl4dmJlbDEgewogIHBhZGRpbmc6IDFyZW07CiAgYmFja2dyb3VuZC1jb2xvcjogdmFyKC0tXzE5ZXoycno3KTsKICBib3JkZXI6IDFweCBzb2xpZCB2YXIoLS1fMTllejJyemUpOwogIGJvcmRlci1yYWRpdXM6IDZweDsKfQouXzE5eHZiZWwyIHsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnowKTsKICBmb250LXNpemU6IDFyZW07CiAgZm9udC13ZWlnaHQ6IDYwMDsKICBtYXJnaW46IDAgMCAwLjc1cmVtIDA7Cn0KLl8xOXh2YmVsMyB7CiAgZGlzcGxheTogZmxleDsKICBmbGV4LWRpcmVjdGlvbjogY29sdW1uOwogIGdhcDogMC43NXJlbTsKfQouXzE5eHZiZWw0IHsKICBkaXNwbGF5OiBmbGV4OwogIGZsZXgtZGlyZWN0aW9uOiBjb2x1bW47CiAgZ2FwOiAwLjM3NXJlbTsKfQouXzE5eHZiZWw1IHsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoxKTsKICBmb250LXNpemU6IDAuODEyNXJlbTsKICBmb250LXdlaWdodDogNjAwOwp9Ci5fMTl4dmJlbDYgewogIGRpc3BsYXk6IGZsZXg7CiAgYWxpZ24taXRlbXM6IGNlbnRlcjsKICBnYXA6IDAuNXJlbTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoxKTsKICBmb250LXNpemU6IDAuODEyNXJlbTsKICBmb250LXdlaWdodDogNjAwOwogIGN1cnNvcjogcG9pbnRlcjsKfQouXzE5eHZiZWw3IHsKICBwYWRkaW5nOiAwLjVyZW0gMC43NXJlbTsKICBiYWNrZ3JvdW5kLWNvbG9yOiB2YXIoLS1fMTllejJyejFhKTsKICBib3JkZXI6IDFweCBzb2xpZCB2YXIoLS1fMTllejJyemspOwogIGJvcmRlci1yYWRpdXM6IDRweDsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoxYik7CiAgZm9udC1zaXplOiAwLjg3NXJlbTsKICBmbGV4OiAxOwp9Ci5fMTl4dmJlbDc6Zm9jdXMgewogIG91dGxpbmU6IG5vbmU7CiAgYm9yZGVyLWNvbG9yOiB2YXIoLS1fMTllejJyemwpOwp9_22_7D = __webpack_require__(17676);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsettings%2FProfilesManager.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA62U0XKjIBSG7_MU53J7gSO2SRr6MDsIR8NWwQFs0uz03TuyZoOGOnZnb-JE4Oc_5_uPmTrbS2Ny-L0BkMp1DX9nUDV4ftlAeBKpLAqvjGYgTNO3elipeceAWmxfNh-b7I8IDSLCNMYyeOP2ByE_6QEvhb3kD0HOaE-cuiADmhXbcHp8e0JVHz2DXZ4P71pua6UZ5JF8kfb4q3deVe9EGO1Rewau4wJJif6EGLzyRtWaKI-tYyBQe7SR7OPXrouHaN_T8r5JdXn2vN9Om7P9L-4DEOe59TMIAB2XUul6uHx_bW3JxWttTa8lSRrfB-OlsRItA9qdwZlGydkujHYRy6XqHYNdd46q2_1TfvJbCFqlyUlJf5wy36-K1Cw8ExCHxzmJ5681aQIkLWbnD98Kwvx2Hk5HsIb17zCjfBW01xS0py5gSeuWK1LMWWVE70INpveN0shAG43RZUn1Jh6lciEtJzskY_iNYnLHUEwVlB6ckKtQcuDvMxdBKEYKqyGsm5wqBYEWCxTuPpSpBMpQ_c3jHMHt_0JKRW_dsNYZdW1QKrhxmyDP6N_uhY4fx8mjsTt2NG9oF6Ysj8OA6TCMsK63LX_Dq1UaYyHEmy6ezE9GdA-eAwcAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsettings_2FProfilesManager_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA62U0XKjIBSG7_MU53J7gSO2SRr6MDsIR8NWwQFs0uz03TuyZoOGOnZnb_JE4Oc_5_uPmTrbS2Ny_L0BkMp1DX9nUDV4ftlAeBKpLAqvjGYgTNO3elipeceAWmxfNh_b7I8IDSLCNMYyeOP2ByE_6QEvhb3kD0HOaE_cuiADmhXbcHp8e0JVHz2DXZ4P71pua6UZ5JF8kfb4q3deVe9EGO1Rewau4wJJif6EGLzyRtWaKI_tYyBQe7SR7OPXrouHaN_T8r5JdXn2vN9Om7P9L_4DEOe59TMIAB2XUul6uHx_bW3JxWttTa8lSRrfB_OlsRItA9qdwZlGydkujHYRy6XqHYNdd46q2_1TfvJbCFqlyUlJf5wy36_K1Cw8ExCHxzmJ5681aQIkLWbnD98Kwvx2Hk5HsIb17zCjfBW01xS0py5gSeuWK1LMWWVE70INpveN0shAG43RZUn1Jh6lciEtJzskY_iNYnLHUEwVlB6ckKtQcuDvMxdBKEYKqyGsm5wqBYEWCxTuPpSpBMpQ_c3jHMHt_0JKRW_dsNYZdW1QKrhxmyDP6N_uhY4fx8mjsTt2NG9oF6Ysj8OA6TCMsK63LX_Dq1UaYyHEmy6ezE9GdA_eAwcAAA_22_7D = __webpack_require__(45125);
;// ./src/components/settings/ProfilesManager.css.ts



var checkboxLabel = '_19xvbel6';
var ProfilesManager_css_container = 'ixrzlo0';
var emptyText = 'ixrzlo4';
var ProfilesManager_css_field = '_19xvbel4';
var formActions = 'ixrzlof';
var formCard = '_19xvbel1';
var formFields = '_19xvbel3';
var formTitle = '_19xvbel2';
var headerRow = 'ixrzlo2';
var ProfilesManager_css_heading = 'ixrzlo1';
var ProfilesManager_css_input = '_19xvbel7';
var ProfilesManager_css_label = '_19xvbel5';
var ProfilesManager_css_loadingText = 'ixrzlo3';
var profileActions = '_19xvbel0';
var profileDesc = 'ixrzlo8';
var profileInfo = 'ixrzlo6';
var profileMeta = 'ixrzlo9';
var profileName = 'ixrzlo7';
var profileRow = 'ixrzlo5';
var ProfilesManager_css_select = 'ixrzloa';
var ProfilesManager_css_tag = 'ixrzloc';
var ProfilesManager_css_tagInputRow = 'ixrzloe';
var ProfilesManager_css_tagList = 'ixrzlob';
var ProfilesManager_css_tagRemove = 'ixrzlod';
;// ./src/components/settings/ProfilesManager.tsx
/* __next_internal_client_entry_do_not_use__ ProfilesManager auto */ 







const emptyForm = {
    name: "",
    description: "",
    program: "",
    autoYes: false,
    tags: [],
    tagInput: ""
};
function ProfilesManager() {
    const [profiles, setProfiles] = (0,react.useState)([]);
    const [loading, setLoading] = (0,react.useState)(true);
    const [error, setError] = (0,react.useState)(null);
    const [success, setSuccess] = (0,react.useState)(null);
    const [showForm, setShowForm] = (0,react.useState)(false);
    const [editingKey, setEditingKey] = (0,react.useState)(null);
    const [form, setForm] = (0,react.useState)({
        ...emptyForm
    });
    const [saving, setSaving] = (0,react.useState)(false);
    const clientRef = (0,react.useRef)(null);
    const loadProfiles = (0,react.useCallback)(async ()=>{
        if (!clientRef.current) return;
        try {
            setLoading(true);
            setError(null);
            const response = await clientRef.current.getSessionDefaults({});
            const defaults = response.defaults;
            if (defaults) {
                const list = Object.entries(defaults.profiles).map((param)=>{
                    let [key, profile] = param;
                    return {
                        key,
                        profile
                    };
                });
                setProfiles(list);
            }
        } catch (err) {
            setError("Failed to load profiles: ".concat(err));
        } finally{
            setLoading(false);
        }
    }, []);
    (0,react.useEffect)(()=>{
        const transport = (0,connect_transport/* createConnectTransport */.w)({
            baseUrl: (0,config/* getApiBaseUrl */.h)()
        });
        clientRef.current = (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, transport);
        loadProfiles();
    }, [
        loadProfiles
    ]);
    const handleEdit = (key, profile)=>{
        setEditingKey(key);
        setForm({
            name: profile.name,
            description: profile.description,
            program: profile.program,
            autoYes: profile.autoYes,
            tags: [
                ...profile.tags
            ],
            tagInput: ""
        });
        setShowForm(true);
    };
    const handleNewProfile = ()=>{
        setEditingKey(null);
        setForm({
            ...emptyForm
        });
        setShowForm(true);
    };
    const handleCancel = ()=>{
        setShowForm(false);
        setEditingKey(null);
        setForm({
            ...emptyForm
        });
    };
    const handleSave = async ()=>{
        if (!clientRef.current) return;
        if (!form.name.trim()) {
            setError("Profile name is required.");
            return;
        }
        try {
            setSaving(true);
            setError(null);
            setSuccess(null);
            await clientRef.current.upsertProfile({
                profile: {
                    name: form.name.trim(),
                    description: form.description,
                    program: form.program,
                    autoYes: form.autoYes,
                    tags: form.tags,
                    envVars: {},
                    cliFlags: ""
                }
            });
            setSuccess('Profile "'.concat(form.name.trim(), '" saved.'));
            setTimeout(()=>setSuccess(null), 3000);
            setShowForm(false);
            setEditingKey(null);
            setForm({
                ...emptyForm
            });
            await loadProfiles();
        } catch (err) {
            setError("Failed to save profile: ".concat(err));
        } finally{
            setSaving(false);
        }
    };
    const handleDelete = async (key)=>{
        if (!clientRef.current) return;
        if (!confirm('Delete profile "'.concat(key, '"?'))) return;
        try {
            setError(null);
            setSuccess(null);
            await clientRef.current.deleteProfile({
                name: key
            });
            setSuccess('Profile "'.concat(key, '" deleted.'));
            setTimeout(()=>setSuccess(null), 3000);
            await loadProfiles();
        } catch (err) {
            setError("Failed to delete profile: ".concat(err));
        }
    };
    const handleAddTag = ()=>{
        const trimmed = form.tagInput.trim();
        if (trimmed && !form.tags.includes(trimmed)) {
            setForm({
                ...form,
                tags: [
                    ...form.tags,
                    trimmed
                ],
                tagInput: ""
            });
        } else {
            setForm({
                ...form,
                tagInput: ""
            });
        }
    };
    const handleRemoveTag = (tag)=>{
        setForm({
            ...form,
            tags: form.tags.filter((t)=>t !== tag)
        });
    };
    const handleTagKeyDown = (e)=>{
        if (e.key === "Enter") {
            e.preventDefault();
            handleAddTag();
        }
    };
    if (loading) {
        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            className: ProfilesManager_css_container,
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                    className: ProfilesManager_css_heading,
                    children: "Profiles"
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: ProfilesManager_css_loadingText,
                    children: "Loading..."
                })
            ]
        });
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: ProfilesManager_css_container,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: headerRow,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                        className: ProfilesManager_css_heading,
                        children: "Profiles"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: "btn btn-primary",
                        onClick: handleNewProfile,
                        children: "New Profile"
                    })
                ]
            }),
            error && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: "alert alert-error",
                children: error
            }),
            success && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: "alert alert-success",
                children: success
            }),
            profiles.length === 0 && !showForm && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: emptyText,
                children: "No profiles configured."
            }),
            profiles.map((param)=>{
                let { key, profile } = param;
                return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: profileRow,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: profileInfo,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    className: profileName,
                                    children: profile.name
                                }),
                                profile.description && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    className: profileDesc,
                                    children: profile.description
                                }),
                                profile.program && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                    className: profileMeta,
                                    children: [
                                        "Program: ",
                                        profile.program
                                    ]
                                }),
                                profile.autoYes && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    className: profileMeta,
                                    children: "Auto-yes: on"
                                }),
                                profile.tags.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                    className: profileMeta,
                                    children: [
                                        "Tags: ",
                                        profile.tags.join(", ")
                                    ]
                                })
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: profileActions,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    type: "button",
                                    className: "btn btn-secondary",
                                    onClick: ()=>handleEdit(key, profile),
                                    children: "Edit"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    type: "button",
                                    className: "btn btn-danger",
                                    onClick: ()=>handleDelete(key),
                                    children: "Delete"
                                })
                            ]
                        })
                    ]
                }, key);
            }),
            showForm && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: formCard,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                        className: formTitle,
                        children: editingKey ? "Edit Profile: ".concat(editingKey) : "New Profile"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: formFields,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: ProfilesManager_css_field,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                        className: ProfilesManager_css_label,
                                        htmlFor: "profile-name",
                                        children: "Name *"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                        id: "profile-name",
                                        type: "text",
                                        className: ProfilesManager_css_input,
                                        placeholder: "e.g. fast-mode",
                                        value: form.name,
                                        onChange: (e)=>setForm({
                                                ...form,
                                                name: e.target.value
                                            }),
                                        disabled: !!editingKey
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: ProfilesManager_css_field,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                        className: ProfilesManager_css_label,
                                        htmlFor: "profile-desc",
                                        children: "Description"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                        id: "profile-desc",
                                        type: "text",
                                        className: ProfilesManager_css_input,
                                        placeholder: "Short description",
                                        value: form.description,
                                        onChange: (e)=>setForm({
                                                ...form,
                                                description: e.target.value
                                            })
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: ProfilesManager_css_field,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                        className: ProfilesManager_css_label,
                                        htmlFor: "profile-program",
                                        children: "Program"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("select", {
                                        id: "profile-program",
                                        className: ProfilesManager_css_select,
                                        value: form.program,
                                        onChange: (e)=>setForm({
                                                ...form,
                                                program: e.target.value
                                            }),
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                                value: "",
                                                children: "Default"
                                            }),
                                            programs/* PROGRAMS */.i.map((p)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                                    value: p.value,
                                                    children: p.label
                                                }, p.value))
                                        ]
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: ProfilesManager_css_field,
                                children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                    className: checkboxLabel,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                            type: "checkbox",
                                            checked: form.autoYes,
                                            onChange: (e)=>setForm({
                                                    ...form,
                                                    autoYes: e.target.checked
                                                })
                                        }),
                                        "Auto-yes"
                                    ]
                                })
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: ProfilesManager_css_field,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                        className: ProfilesManager_css_label,
                                        children: "Tags"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                        className: ProfilesManager_css_tagList,
                                        children: form.tags.map((t)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                className: ProfilesManager_css_tag,
                                                children: [
                                                    t,
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                        type: "button",
                                                        className: ProfilesManager_css_tagRemove,
                                                        onClick: ()=>handleRemoveTag(t),
                                                        "aria-label": "Remove tag ".concat(t),
                                                        children: "x"
                                                    })
                                                ]
                                            }, t))
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        className: ProfilesManager_css_tagInputRow,
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                type: "text",
                                                className: ProfilesManager_css_input,
                                                placeholder: "Add a tag...",
                                                value: form.tagInput,
                                                onChange: (e)=>setForm({
                                                        ...form,
                                                        tagInput: e.target.value
                                                    }),
                                                onKeyDown: handleTagKeyDown
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                type: "button",
                                                className: "btn btn-secondary",
                                                onClick: handleAddTag,
                                                children: "Add"
                                            })
                                        ]
                                    })
                                ]
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: formActions,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: "btn btn-primary",
                                onClick: handleSave,
                                disabled: saving,
                                children: saving ? "Saving..." : "Save"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: "btn btn-secondary",
                                onClick: handleCancel,
                                children: "Cancel"
                            })
                        ]
                    })
                ]
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsettings%2FDirectoryRulesManager.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA61V3XKzIBS8z1Ocy_aCjJh_8jDfoBwNDYKDmL9v-u4dqUnR2NRmehMnclj27NnF6SF_O88OEfyfAAhZlYqfGWQKT9sJ-CcR0mLqpNEMUqPqQjcrOS8ZUIvFdvI-mX6CUA-SGmUsgwO3L4T8oxu8xPYSvXo4ox2p5AUZ0Gm88Lvbt0eU-c4xWEZR867gNpeaQRTAx8Mc3-rKyexMUqMdasegKnmKJEF3RPRcuZK5JtJhUTFIUTu0Aezse9bxa1A3f1zX6S6arleLrjiLP2HvB1I5bl1vCAAlF0LqvDl8dZU24ek-t6bWggwSX3niibECLQNanqAySopeFQZVxHIh64rBsjwF3S2f8k_0ZYJCanKUwu26M1-NslTPPJ1BbGarjtEyXkh1ZlAYbbzWzcrRWEESi3zPwD8IVyqgsf7V7Puj3_jdrX6DIDQKnca_P43eBen-uMRvD-zQrP_GFZSPssV-yBbz0g9-GDcZkZOEZSatK9-DqZ2SGhloo3H7g4YqlDB90o83jZ5J0_r5NM07aRIP2B9tw7T5DWjP-ipiF0HqRkdyBRq8EO8zGWgQtx4abaFxN0s2pAWNH3howP9rGve6zz4Dd-PYN9DX_weRTmtbNWulkVeBhmIXygTRlN7U84rv2puJhuzYzhzQ_hDyW3k-bIZ2WNfTHn_jdqMw2kaIM2EW3j8AudcODiMIAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsettings_2FDirectoryRulesManager_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA61V3XKzIBS8z1Ocy_aCjJh_8jDfoBwNDYKDmL9v_u4dqUnR2NRmehMnclj27NnF6SF_O88OEfyfAAhZlYqfGWQKT9sJ_CcR0mLqpNEMUqPqQjcrOS8ZUIvFdvI_mX6CUA_SGmUsgwO3L4T8oxu8xPYSvXo4ox2p5AUZ0Gm88Lvbt0eU_c4xWEZR867gNpeaQRTAx8Mc3_rKyexMUqMdasegKnmKJEF3RPRcuZK5JtJhUTFIUTu0Aezse9bxa1A3f1zX6S6arleLrjiLP2HvB1I5bl1vCAAlF0LqvDl8dZU24ek_t6bWggwSX3niibECLQNanqAySopeFQZVxHIh64rBsjwF3S2f8k_0ZYJCanKUwu26M1_NslTPPJ1BbGarjtEyXkh1ZlAYbbzWzcrRWEESi3zPwD8IVyqgsf7V7Puj3_jdrX6DIDQKnca_P43eBen_uMRvD_zQrP_GFZSPssV_yBbz0g9_GDcZkZOEZSatK9_DqZ2SGhloo3H7g4YqlDB90o83jZ5J0_r5NM07aRIP2B9tw7T5DWjP_ipiF0HqRkdyBRq8EO8zGWgQtx4abaFxN0s2pAWNH3howP9rGve6zz4Dd_PYN9DX_weRTmtbNWulkVeBhmIXygTRlN7U84rv2puJhuzYzhzQ_hDyW3k_bIZ2WNfTHn_jdqMw2kaIM2EW3j8AudcODiMIAAA_22_7D = __webpack_require__(91667);
;// ./src/components/settings/DirectoryRulesManager.css.ts



var DirectoryRulesManager_css_checkboxLabel = '_19xvbel6';
var DirectoryRulesManager_css_container = 'vgjy3v0';
var DirectoryRulesManager_css_emptyText = 'vgjy3v4';
var DirectoryRulesManager_css_field = '_19xvbel4';
var fieldError = 'vgjy3va';
var DirectoryRulesManager_css_formActions = 'vgjy3vh';
var DirectoryRulesManager_css_formCard = '_19xvbel1';
var DirectoryRulesManager_css_formFields = '_19xvbel3';
var DirectoryRulesManager_css_formTitle = '_19xvbel2';
var DirectoryRulesManager_css_headerRow = 'vgjy3v2';
var DirectoryRulesManager_css_heading = 'vgjy3v1';
var DirectoryRulesManager_css_input = '_19xvbel7';
var inputError = 'vgjy3v9';
var DirectoryRulesManager_css_label = '_19xvbel5';
var DirectoryRulesManager_css_loadingText = 'vgjy3v3';
var overridesSection = 'vgjy3vc';
var ruleActions = '_19xvbel0';
var ruleInfo = 'vgjy3v6';
var ruleMeta = 'vgjy3v8';
var rulePath = 'vgjy3v7';
var ruleRow = 'vgjy3v5';
var DirectoryRulesManager_css_select = 'vgjy3vb';
var DirectoryRulesManager_css_tag = 'vgjy3ve';
var DirectoryRulesManager_css_tagInputRow = 'vgjy3vg';
var DirectoryRulesManager_css_tagList = 'vgjy3vd';
var DirectoryRulesManager_css_tagRemove = 'vgjy3vf';
;// ./src/components/settings/DirectoryRulesManager.tsx
/* __next_internal_client_entry_do_not_use__ DirectoryRulesManager auto */ 








const DirectoryRulesManager_emptyForm = {
    path: "",
    profile: "",
    showOverrides: false,
    overrideProgram: "",
    overrideAutoYes: false,
    overrideTags: [],
    tagInput: ""
};
function DirectoryRulesManager() {
    const [rules, setRules] = (0,react.useState)([]);
    const [profiles, setProfiles] = (0,react.useState)([]);
    const [loading, setLoading] = (0,react.useState)(true);
    const [error, setError] = (0,react.useState)(null);
    const [success, setSuccess] = (0,react.useState)(null);
    const [showForm, setShowForm] = (0,react.useState)(false);
    const [editingPath, setEditingPath] = (0,react.useState)(null);
    const [form, setForm] = (0,react.useState)({
        ...DirectoryRulesManager_emptyForm
    });
    const [saving, setSaving] = (0,react.useState)(false);
    const [pathError, setPathError] = (0,react.useState)(null);
    const clientRef = (0,react.useRef)(null);
    const loadRules = (0,react.useCallback)(async ()=>{
        if (!clientRef.current) return;
        try {
            setLoading(true);
            setError(null);
            const response = await clientRef.current.getSessionDefaults({});
            const defaults = response.defaults;
            if (defaults) {
                setRules([
                    ...defaults.directoryRules
                ]);
                setProfiles(Object.keys(defaults.profiles));
            }
        } catch (err) {
            setError("Failed to load directory rules: ".concat(err));
        } finally{
            setLoading(false);
        }
    }, []);
    (0,react.useEffect)(()=>{
        const transport = (0,connect_transport/* createConnectTransport */.w)({
            baseUrl: (0,config/* getApiBaseUrl */.h)()
        });
        clientRef.current = (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, transport);
        loadRules();
    }, [
        loadRules
    ]);
    const handleEdit = (rule)=>{
        var _rule_overrides, _rule_overrides1, _rule_overrides2;
        setEditingPath(rule.path);
        var _rule_overrides_program, _rule_overrides_autoYes, _rule_overrides_tags;
        setForm({
            path: rule.path,
            profile: rule.profile,
            showOverrides: !!rule.overrides,
            overrideProgram: (_rule_overrides_program = (_rule_overrides = rule.overrides) === null || _rule_overrides === void 0 ? void 0 : _rule_overrides.program) !== null && _rule_overrides_program !== void 0 ? _rule_overrides_program : "",
            overrideAutoYes: (_rule_overrides_autoYes = (_rule_overrides1 = rule.overrides) === null || _rule_overrides1 === void 0 ? void 0 : _rule_overrides1.autoYes) !== null && _rule_overrides_autoYes !== void 0 ? _rule_overrides_autoYes : false,
            overrideTags: [
                ...(_rule_overrides_tags = (_rule_overrides2 = rule.overrides) === null || _rule_overrides2 === void 0 ? void 0 : _rule_overrides2.tags) !== null && _rule_overrides_tags !== void 0 ? _rule_overrides_tags : []
            ],
            tagInput: ""
        });
        setPathError(null);
        setShowForm(true);
    };
    const handleNewRule = ()=>{
        setEditingPath(null);
        setForm({
            ...DirectoryRulesManager_emptyForm
        });
        setPathError(null);
        setShowForm(true);
    };
    const handleCancel = ()=>{
        setShowForm(false);
        setEditingPath(null);
        setForm({
            ...DirectoryRulesManager_emptyForm
        });
        setPathError(null);
    };
    const validatePath = (path)=>{
        if (!path.trim()) return "Path is required.";
        if (!path.startsWith("/")) return "Path must be an absolute path (start with /).";
        return null;
    };
    const handleSave = async ()=>{
        if (!clientRef.current) return;
        const pathErr = validatePath(form.path);
        if (pathErr) {
            setPathError(pathErr);
            return;
        }
        setPathError(null);
        const overrides = form.showOverrides ? {
            program: form.overrideProgram,
            autoYes: form.overrideAutoYes,
            tags: form.overrideTags,
            name: "",
            description: "",
            envVars: {},
            cliFlags: ""
        } : undefined;
        try {
            setSaving(true);
            setError(null);
            setSuccess(null);
            await clientRef.current.upsertDirectoryRule({
                rule: {
                    path: form.path.trim(),
                    profile: form.profile,
                    overrides
                }
            });
            setSuccess('Rule for "'.concat(form.path.trim(), '" saved.'));
            setTimeout(()=>setSuccess(null), 3000);
            setShowForm(false);
            setEditingPath(null);
            setForm({
                ...DirectoryRulesManager_emptyForm
            });
            await loadRules();
        } catch (err) {
            setError("Failed to save rule: ".concat(err));
        } finally{
            setSaving(false);
        }
    };
    const handleDelete = async (path)=>{
        if (!clientRef.current) return;
        if (!confirm('Delete rule for "'.concat(path, '"?'))) return;
        try {
            setError(null);
            setSuccess(null);
            await clientRef.current.deleteDirectoryRule({
                path
            });
            setSuccess('Rule for "'.concat(path, '" deleted.'));
            setTimeout(()=>setSuccess(null), 3000);
            await loadRules();
        } catch (err) {
            setError("Failed to delete rule: ".concat(err));
        }
    };
    const handleAddTag = ()=>{
        const trimmed = form.tagInput.trim();
        if (trimmed && !form.overrideTags.includes(trimmed)) {
            setForm({
                ...form,
                overrideTags: [
                    ...form.overrideTags,
                    trimmed
                ],
                tagInput: ""
            });
        } else {
            setForm({
                ...form,
                tagInput: ""
            });
        }
    };
    const handleRemoveTag = (tag)=>{
        setForm({
            ...form,
            overrideTags: form.overrideTags.filter((t)=>t !== tag)
        });
    };
    const handleTagKeyDown = (e)=>{
        if (e.key === "Enter") {
            e.preventDefault();
            handleAddTag();
        }
    };
    if (loading) {
        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            className: DirectoryRulesManager_css_container,
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                    className: DirectoryRulesManager_css_heading,
                    children: "Directory Rules"
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: DirectoryRulesManager_css_loadingText,
                    children: "Loading..."
                })
            ]
        });
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: DirectoryRulesManager_css_container,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: DirectoryRulesManager_css_headerRow,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                        className: DirectoryRulesManager_css_heading,
                        children: "Directory Rules"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: "btn btn-primary",
                        onClick: handleNewRule,
                        children: "New Rule"
                    })
                ]
            }),
            error && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: "alert alert-error",
                children: error
            }),
            success && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: "alert alert-success",
                children: success
            }),
            rules.length === 0 && !showForm && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: DirectoryRulesManager_css_emptyText,
                children: "No directory rules configured. Rules auto-populate form fields based on working directory."
            }),
            rules.map((rule)=>{
                var _rule_overrides, _rule_overrides1, _rule_overrides_tags, _rule_overrides2;
                var _rule_overrides_tags_length;
                return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: ruleRow,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: ruleInfo,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    className: rulePath,
                                    children: rule.path
                                }),
                                rule.profile && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                    className: ruleMeta,
                                    children: [
                                        "Profile: ",
                                        rule.profile
                                    ]
                                }),
                                ((_rule_overrides = rule.overrides) === null || _rule_overrides === void 0 ? void 0 : _rule_overrides.program) && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                    className: ruleMeta,
                                    children: [
                                        "Program: ",
                                        rule.overrides.program
                                    ]
                                }),
                                ((_rule_overrides1 = rule.overrides) === null || _rule_overrides1 === void 0 ? void 0 : _rule_overrides1.autoYes) && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    className: ruleMeta,
                                    children: "Auto-yes: on"
                                }),
                                ((_rule_overrides_tags_length = (_rule_overrides2 = rule.overrides) === null || _rule_overrides2 === void 0 ? void 0 : (_rule_overrides_tags = _rule_overrides2.tags) === null || _rule_overrides_tags === void 0 ? void 0 : _rule_overrides_tags.length) !== null && _rule_overrides_tags_length !== void 0 ? _rule_overrides_tags_length : 0) > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                    className: ruleMeta,
                                    children: [
                                        "Tags: ",
                                        rule.overrides.tags.join(", ")
                                    ]
                                })
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: ruleActions,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    type: "button",
                                    className: "btn btn-secondary",
                                    onClick: ()=>handleEdit(rule),
                                    children: "Edit"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    type: "button",
                                    className: "btn btn-danger",
                                    onClick: ()=>handleDelete(rule.path),
                                    children: "Delete"
                                })
                            ]
                        })
                    ]
                }, rule.path);
            }),
            showForm && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: DirectoryRulesManager_css_formCard,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                        className: DirectoryRulesManager_css_formTitle,
                        children: editingPath ? "Edit Rule: ".concat(editingPath) : "New Directory Rule"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: DirectoryRulesManager_css_formFields,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: DirectoryRulesManager_css_field,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                        className: DirectoryRulesManager_css_label,
                                        htmlFor: "rule-path",
                                        children: "Directory Path *"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)(RepoPathInput/* RepoPathInput */.E, {
                                        id: "rule-path",
                                        placeholder: "/Users/you/projects/myrepo",
                                        value: form.path,
                                        onChange: (v)=>{
                                            setForm({
                                                ...form,
                                                path: v
                                            });
                                            if (pathError) setPathError(validatePath(v));
                                        },
                                        disabled: !!editingPath,
                                        error: pathError || undefined
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: DirectoryRulesManager_css_field,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                        className: DirectoryRulesManager_css_label,
                                        htmlFor: "rule-profile",
                                        children: "Profile (optional)"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("select", {
                                        id: "rule-profile",
                                        className: DirectoryRulesManager_css_select,
                                        value: form.profile,
                                        onChange: (e)=>setForm({
                                                ...form,
                                                profile: e.target.value
                                            }),
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                                value: "",
                                                children: "None"
                                            }),
                                            profiles.map((p)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                                    value: p,
                                                    children: p
                                                }, p))
                                        ]
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: DirectoryRulesManager_css_field,
                                children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                    className: DirectoryRulesManager_css_checkboxLabel,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                            type: "checkbox",
                                            checked: form.showOverrides,
                                            onChange: (e)=>setForm({
                                                    ...form,
                                                    showOverrides: e.target.checked
                                                })
                                        }),
                                        "Add field overrides"
                                    ]
                                })
                            }),
                            form.showOverrides && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: overridesSection,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        className: DirectoryRulesManager_css_field,
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                                className: DirectoryRulesManager_css_label,
                                                htmlFor: "rule-program",
                                                children: "Override Program"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("select", {
                                                id: "rule-program",
                                                className: DirectoryRulesManager_css_select,
                                                value: form.overrideProgram,
                                                onChange: (e)=>setForm({
                                                        ...form,
                                                        overrideProgram: e.target.value
                                                    }),
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                                        value: "",
                                                        children: "Default"
                                                    }),
                                                    programs/* PROGRAMS */.i.map((p)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                                            value: p.value,
                                                            children: p.label
                                                        }, p.value))
                                                ]
                                            })
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                        className: DirectoryRulesManager_css_field,
                                        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                            className: DirectoryRulesManager_css_checkboxLabel,
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                    type: "checkbox",
                                                    checked: form.overrideAutoYes,
                                                    onChange: (e)=>setForm({
                                                            ...form,
                                                            overrideAutoYes: e.target.checked
                                                        })
                                                }),
                                                "Auto-yes"
                                            ]
                                        })
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        className: DirectoryRulesManager_css_field,
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                                className: DirectoryRulesManager_css_label,
                                                children: "Override Tags"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                                className: DirectoryRulesManager_css_tagList,
                                                children: form.overrideTags.map((t)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                        className: DirectoryRulesManager_css_tag,
                                                        children: [
                                                            t,
                                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                                type: "button",
                                                                className: DirectoryRulesManager_css_tagRemove,
                                                                onClick: ()=>handleRemoveTag(t),
                                                                "aria-label": "Remove tag ".concat(t),
                                                                children: "x"
                                                            })
                                                        ]
                                                    }, t))
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                className: DirectoryRulesManager_css_tagInputRow,
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                        type: "text",
                                                        className: DirectoryRulesManager_css_input,
                                                        placeholder: "Add a tag...",
                                                        value: form.tagInput,
                                                        onChange: (e)=>setForm({
                                                                ...form,
                                                                tagInput: e.target.value
                                                            }),
                                                        onKeyDown: handleTagKeyDown
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                        type: "button",
                                                        className: "btn btn-secondary",
                                                        onClick: handleAddTag,
                                                        children: "Add"
                                                    })
                                                ]
                                            })
                                        ]
                                    })
                                ]
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: DirectoryRulesManager_css_formActions,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: "btn btn-primary",
                                onClick: handleSave,
                                disabled: saving,
                                children: saving ? "Saving..." : "Save"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: "btn btn-secondary",
                                onClick: handleCancel,
                                children: "Cancel"
                            })
                        ]
                    })
                ]
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/gen/session/v1/types_pb.ts
var types_pb = __webpack_require__(81432);
// EXTERNAL MODULE: ./src/lib/omnibar/detectors/AliasDetector.ts
var AliasDetector = __webpack_require__(63251);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsettings%2FAliasesManager.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA9VW226jMBB9z1f4cfPgCEOTNO7HrAw4xA3Y1JjcVvvvK0wCxjGUpLTSqlKrpGZmzmUOXqREnEoP_JkBELMiT8kZg21KT28zoP_CmEkaKSY4BpFIy4xX_0lIjgGSNHub_Z0tdA2ka0QiFRKDA5G_IPyNNvTiy4s319UEV7BgF4oBWvhL_fD12yNlyU5hsPK86ruMyIRxDLy2uu-e8L0sFNueYSS4olxhUOQkojCk6kipnpSkLOGQKZoVGESUKyrbqkH_zP68PfYyfKwDzVu8rpcdYpaTjK61KBSRyuIfgJzEMeNJ1Xt9ozUk0T6RouQxdM691nOHQsZUYoDyEyhEymLrFDVOQUliVhYYrPJTC271lHO8Vv-McXhksdp15F6PMpNlm44Km8CS4bW_JHKIiPzu45uHPGD1Jm6WrlwEjWq1yDvJ-L5DR6ifb3RGPyxy9Mhqj9nr6qfmyEQZP-kli2z6ZBnbMNvnDOOCfiuZuEdzZlQz163sJJMAEJWyqOrkgnXDcNc1Wd34kUxBZJTf9i6_veSnfoxhf8jWumKADBx4K6Ky0GhEqVLGKQZccGq0dfZJjchn_yMZzfBTMPA-sEZHWVmz-j2wP_tuAcarOeA4zzcONhTwrxKMVmBc_G1dCiB_QIK7zHO8LlINvh3R5r_9PPBKuVtV91vGZAl4i3b9NeG76_obG5LinThQOZBvnmGEbPDldes1eM_io0pcUUAlXLEunop1i9aLgSsfGMrx2CcQP3S1Joy-NyC6R4LV_LO4aAxiATvP3VANm1m9QoPBjylyRlqb0sMWGsUW8r5IV-_iTccgub_AWicSg55iHD1Bhx6_j57ga_QsB2OpBw2b2IHEYEc9fqFyduu9FJUPXPudOtd33jrT7qC3fQ6fhPFkjY7fDegff_zV1VIQAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsettings_2FAliasesManager_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA9VW226jMBB9z1f4cfPgCEOTNO7HrAw4xA3Y1JjcVvvvK0wCxjGUpLTSqlKrpGZmzmUOXqREnEoP_JkBELMiT8kZg21KT28zoP_CmEkaKSY4BpFIy4xX_0lIjgGSNHub_Z0tdA2ka0QiFRKDA5G_IPyNNvTiy4s319UEV7BgF4oBWvhL_fD12yNlyU5hsPK86ruMyIRxDLy2uu_e8L0sFNueYSS4olxhUOQkojCk6kipnpSkLOGQKZoVGESUKyrbqkH_zP68PfYyfKwDzVu8rpcdYpaTjK61KBSRyuIfgJzEMeNJ1Xt9ozUk0T6RouQxdM691nOHQsZUYoDyEyhEymLrFDVOQUliVhYYrPJTC271lHO8Vv_McXhksdp15F6PMpNlm44Km8CS4bW_JHKIiPzu45uHPGD1Jm6WrlwEjWq1yDvJ_L5DR6ifb3RGPyxy9Mhqj9nr6qfmyEQZP_kli2z6ZBnbMNvnDOOCfiuZuEdzZlQz163sJJMAEJWyqOrkgnXDcNc1Wd34kUxBZJTf9i6_veSnfoxhf8jWumKADBx4K6Ky0GhEqVLGKQZccGq0dfZJjchn_yMZzfBTMPA_sEZHWVmz_j2wP_tuAcarOeA4zzcONhTwrxKMVmBc_G1dCiB_QIK7zHO8LlINvh3R5r_9PPBKuVtV91vGZAl4i3b9NeG76_obG5LinThQOZBvnmGEbPDldes1eM_io0pcUUAlXLEunop1i9aLgSsfGMrx2CcQP3S1Joy_NyC6R4LV_LO4aAxiATvP3VANm1m9QoPBjylyRlqb0sMWGsUW8r5IV_iTccgub_AWicSg55iHD1Bhx6_j57ga_QsB2OpBw2b2IHEYEc9fqFyduu9FJUPXPudOtd33jrT7qC3fQ6fhPFkjY7fDegff_zV1VIQAAA_22_7D = __webpack_require__(84101);
;// ./src/components/settings/AliasesManager.css.ts


var advancedToggle = 'laoxut';
var aliasActions = 'laoxua';
var aliasDesc = 'laoxu8';
var aliasInfo = 'laoxu6';
var aliasMeta = 'laoxu9';
var aliasName = 'laoxu7';
var aliasRow = 'laoxu5';
var AliasesManager_css_checkboxLabel = 'laoxug';
var confirmDeleteBtn = 'laoxus';
var AliasesManager_css_container = 'laoxu0';
var AliasesManager_css_deleteBtn = 'laoxur';
var AliasesManager_css_emptyText = 'laoxu4';
var envVarInput = 'laoxuq';
var AliasesManager_css_envVarRow = 'laoxup';
var AliasesManager_css_envVarTable = 'laoxuo';
var AliasesManager_css_field = 'laoxue';
var AliasesManager_css_fieldError = 'laoxuv';
var AliasesManager_css_formActions = 'laoxun';
var AliasesManager_css_formCard = 'laoxub';
var AliasesManager_css_formFields = 'laoxud';
var AliasesManager_css_formTitle = 'laoxuc';
var groupHint = 'laoxuw';
var AliasesManager_css_headerRow = 'laoxu2';
var AliasesManager_css_heading = 'laoxu1';
var AliasesManager_css_input = 'laoxuh';
var AliasesManager_css_label = 'laoxuf';
var AliasesManager_css_loadingText = 'laoxu3';
var previewHint = 'laoxuu';
var AliasesManager_css_select = 'laoxui';
var AliasesManager_css_tag = 'laoxuk';
var AliasesManager_css_tagInputRow = 'laoxum';
var AliasesManager_css_tagList = 'laoxuj';
var AliasesManager_css_tagRemove = 'laoxul';
;// ./src/components/settings/AliasesManager.tsx
/* __next_internal_client_entry_do_not_use__ AliasesManager auto */ 









const SESSION_TYPE_OPTIONS = [
    {
        value: types_pb/* SessionType */.dF.UNSPECIFIED,
        label: "Default (directory)"
    },
    {
        value: types_pb/* SessionType */.dF.DIRECTORY,
        label: "Directory"
    },
    {
        value: types_pb/* SessionType */.dF.NEW_WORKTREE,
        label: "New Worktree"
    },
    {
        value: types_pb/* SessionType */.dF.EXISTING_WORKTREE,
        label: "Existing Worktree"
    },
    {
        value: types_pb/* SessionType */.dF.ONE_OFF,
        label: "One-off"
    }
];

const AliasesManager_emptyForm = {
    name: "",
    description: "",
    group: "",
    path: "",
    profile: "",
    program: "",
    autoYes: false,
    tags: [],
    tagInput: "",
    envVars: [],
    cliFlags: "",
    sessionType: types_pb/* SessionType */.dF.UNSPECIFIED,
    namePrefix: "",
    showAdvanced: false
};
function AliasesManager() {
    const [aliases, setAliases] = (0,react.useState)([]);
    const [loading, setLoading] = (0,react.useState)(true);
    const [error, setError] = (0,react.useState)(null);
    const [nameError, setNameError] = (0,react.useState)(null);
    const [success, setSuccess] = (0,react.useState)(null);
    const [showForm, setShowForm] = (0,react.useState)(false);
    const [editingName, setEditingName] = (0,react.useState)(null);
    const [form, setForm] = (0,react.useState)({
        ...AliasesManager_emptyForm
    });
    const [saving, setSaving] = (0,react.useState)(false);
    const [pendingDeleteName, setPendingDeleteName] = (0,react.useState)(null);
    const clientRef = (0,react.useRef)(null);
    const nameInputRef = (0,react.useRef)(null);
    const pendingDeleteTimerRef = (0,react.useRef)(null);
    const successBannerTimerRef = (0,react.useRef)(null);
    // Timer cleanup on unmount
    (0,react.useEffect)(()=>{
        return ()=>{
            if (pendingDeleteTimerRef.current) clearTimeout(pendingDeleteTimerRef.current);
            if (successBannerTimerRef.current) clearTimeout(successBannerTimerRef.current);
        };
    }, []);
    const loadAliases = (0,react.useCallback)(async ()=>{
        if (!clientRef.current) return;
        try {
            setLoading(true);
            setError(null);
            const response = await clientRef.current.listAliases({});
            var _response_aliases;
            setAliases((_response_aliases = response.aliases) !== null && _response_aliases !== void 0 ? _response_aliases : []);
        } catch (err) {
            setError("Failed to load aliases: ".concat(err));
        } finally{
            setLoading(false);
        }
    }, []);
    (0,react.useEffect)(()=>{
        const transport = (0,connect_transport/* createConnectTransport */.w)({
            baseUrl: (0,config/* getApiBaseUrl */.h)()
        });
        clientRef.current = (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, transport);
        loadAliases();
    }, [
        loadAliases
    ]);
    // Focus name input when creating new alias
    (0,react.useEffect)(()=>{
        if (showForm && editingName === null && nameInputRef.current) {
            nameInputRef.current.focus();
        }
    }, [
        showForm,
        editingName
    ]);
    const handleEdit = (alias)=>{
        setEditingName(alias.name);
        var _alias_envVars;
        const envVarsList = Object.entries((_alias_envVars = alias.envVars) !== null && _alias_envVars !== void 0 ? _alias_envVars : {}).map((param)=>{
            let [key, value] = param;
            return {
                key,
                value
            };
        });
        const hasAdvanced = envVarsList.length > 0 || !!alias.cliFlags;
        setForm({
            name: alias.name,
            description: alias.description,
            group: alias.group,
            path: alias.path,
            profile: alias.profile,
            program: alias.program,
            autoYes: alias.autoYes,
            tags: [
                ...alias.tags
            ],
            tagInput: "",
            envVars: envVarsList,
            cliFlags: alias.cliFlags,
            sessionType: alias.sessionType,
            namePrefix: alias.namePrefix,
            showAdvanced: hasAdvanced
        });
        setNameError(null);
        setShowForm(true);
    };
    const handleNewAlias = ()=>{
        setEditingName(null);
        setForm({
            ...AliasesManager_emptyForm
        });
        setNameError(null);
        setShowForm(true);
    };
    const handleCancel = ()=>{
        setShowForm(false);
        setEditingName(null);
        setNameError(null);
        setForm({
            ...AliasesManager_emptyForm
        });
    };
    const handleSave = async ()=>{
        if (!clientRef.current) return;
        const trimmedName = form.name.trim();
        if (!trimmedName) {
            setNameError("Name is required.");
            return;
        }
        if (!AliasDetector/* ALIAS_NAME_RE */.P.test(trimmedName)) {
            setNameError("Name may only contain letters, digits, hyphens, and underscores.");
            return;
        }
        // Case-insensitive collision check in create mode only
        if (editingName === null) {
            const collision = aliases.find((a)=>a.name.toLowerCase() === trimmedName.toLowerCase());
            if (collision) {
                setNameError('An alias named "@'.concat(collision.name, '" already exists. Edit it instead.'));
                return;
            }
        }
        // Build envVars map (skip blank keys)
        const envVarsMap = {};
        for (const ev of form.envVars){
            if (ev.key.trim()) {
                envVarsMap[ev.key.trim()] = ev.value;
            }
        }
        try {
            setSaving(true);
            setError(null);
            setSuccess(null);
            await clientRef.current.upsertAlias({
                alias: {
                    name: trimmedName,
                    description: form.description,
                    group: form.group,
                    path: form.path,
                    profile: form.profile,
                    program: form.program,
                    autoYes: form.autoYes,
                    tags: form.tags,
                    envVars: envVarsMap,
                    cliFlags: form.cliFlags,
                    sessionType: form.sessionType,
                    namePrefix: form.namePrefix
                }
            });
            if (successBannerTimerRef.current) clearTimeout(successBannerTimerRef.current);
            setSuccess('Alias "@'.concat(trimmedName, '" saved.'));
            successBannerTimerRef.current = setTimeout(()=>setSuccess(null), 3000);
            setShowForm(false);
            setEditingName(null);
            setNameError(null);
            setForm({
                ...AliasesManager_emptyForm
            });
            await loadAliases();
        } catch (err) {
            setError("Failed to save alias: ".concat(err));
        } finally{
            setSaving(false);
        }
    };
    const handleDeleteClick = (name)=>{
        if (pendingDeleteTimerRef.current) clearTimeout(pendingDeleteTimerRef.current);
        setPendingDeleteName(name);
        pendingDeleteTimerRef.current = setTimeout(()=>{
            setPendingDeleteName(null);
        }, 3000);
    };
    const handleDeleteConfirm = async (name)=>{
        if (!clientRef.current) return;
        if (pendingDeleteTimerRef.current) clearTimeout(pendingDeleteTimerRef.current);
        setPendingDeleteName(null);
        try {
            setError(null);
            setSuccess(null);
            await clientRef.current.deleteAlias({
                name
            });
            if (successBannerTimerRef.current) clearTimeout(successBannerTimerRef.current);
            setSuccess('Alias "@'.concat(name, '" deleted.'));
            successBannerTimerRef.current = setTimeout(()=>setSuccess(null), 3000);
            await loadAliases();
        } catch (err) {
            setError("Failed to delete alias: ".concat(err));
        }
    };
    const handleAddTag = ()=>{
        const trimmed = form.tagInput.trim();
        if (trimmed && !form.tags.includes(trimmed)) {
            setForm({
                ...form,
                tags: [
                    ...form.tags,
                    trimmed
                ],
                tagInput: ""
            });
        } else {
            setForm({
                ...form,
                tagInput: ""
            });
        }
    };
    const handleRemoveTag = (t)=>{
        setForm({
            ...form,
            tags: form.tags.filter((tag)=>tag !== t)
        });
    };
    const handleTagKeyDown = (e)=>{
        if (e.key === "Enter") {
            e.preventDefault();
            handleAddTag();
        }
    };
    const handleAddEnvVar = ()=>{
        setForm({
            ...form,
            envVars: [
                ...form.envVars,
                {
                    key: "",
                    value: ""
                }
            ]
        });
    };
    const handleEnvVarChange = (index, fieldName, val)=>{
        const updated = form.envVars.map((ev, i)=>i === index ? {
                ...ev,
                [fieldName]: val
            } : ev);
        setForm({
            ...form,
            envVars: updated
        });
    };
    const handleRemoveEnvVar = (index)=>{
        setForm({
            ...form,
            envVars: form.envVars.filter((_, i)=>i !== index)
        });
    };
    if (loading) {
        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            className: AliasesManager_css_container,
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                    className: AliasesManager_css_heading,
                    children: "Aliases"
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: AliasesManager_css_loadingText,
                    children: "Loading..."
                })
            ]
        });
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: AliasesManager_css_container,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: AliasesManager_css_headerRow,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                        className: AliasesManager_css_heading,
                        children: "Aliases"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: "btn btn-primary",
                        onClick: handleNewAlias,
                        disabled: saving,
                        children: "New Alias"
                    })
                ]
            }),
            error && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: "alert alert-error",
                role: "alert",
                children: error
            }),
            success && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: "alert alert-success",
                role: "status",
                children: success
            }),
            aliases.length === 0 && !showForm && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: AliasesManager_css_emptyText,
                children: "No aliases configured."
            }),
            aliases.map((alias)=>{
                var _SESSION_TYPE_OPTIONS_find;
                var _SESSION_TYPE_OPTIONS_find_label;
                return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: aliasRow,
                    "data-testid": "alias-row-".concat(alias.name),
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: aliasInfo,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                    className: aliasName,
                                    children: [
                                        "@",
                                        alias.name
                                    ]
                                }),
                                alias.description && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    className: aliasDesc,
                                    children: alias.description
                                }),
                                alias.group && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                    className: aliasMeta,
                                    children: [
                                        "Group: ",
                                        alias.group
                                    ]
                                }),
                                alias.path && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                    className: aliasMeta,
                                    children: [
                                        "Path: ",
                                        alias.path
                                    ]
                                }),
                                alias.program && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                    className: aliasMeta,
                                    children: [
                                        "Program: ",
                                        alias.program
                                    ]
                                }),
                                alias.sessionType !== types_pb/* SessionType */.dF.UNSPECIFIED && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                    className: aliasMeta,
                                    children: [
                                        "Type: ",
                                        (_SESSION_TYPE_OPTIONS_find_label = (_SESSION_TYPE_OPTIONS_find = SESSION_TYPE_OPTIONS.find((o)=>o.value === alias.sessionType)) === null || _SESSION_TYPE_OPTIONS_find === void 0 ? void 0 : _SESSION_TYPE_OPTIONS_find.label) !== null && _SESSION_TYPE_OPTIONS_find_label !== void 0 ? _SESSION_TYPE_OPTIONS_find_label : ""
                                    ]
                                }),
                                alias.autoYes && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    className: aliasMeta,
                                    children: "Auto-yes: on"
                                }),
                                alias.tags.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                    className: aliasMeta,
                                    children: [
                                        "Tags: ",
                                        alias.tags.join(", ")
                                    ]
                                })
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: aliasActions,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    type: "button",
                                    className: "btn btn-secondary",
                                    onClick: ()=>handleEdit(alias),
                                    children: "Edit"
                                }),
                                pendingDeleteName === alias.name ? /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    type: "button",
                                    className: confirmDeleteBtn,
                                    onClick: ()=>handleDeleteConfirm(alias.name),
                                    "aria-label": "Confirm delete alias ".concat(alias.name),
                                    "data-testid": "alias-confirm-delete-".concat(alias.name),
                                    children: "Confirm delete?"
                                }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    type: "button",
                                    className: AliasesManager_css_deleteBtn,
                                    onClick: ()=>handleDeleteClick(alias.name),
                                    "aria-label": "Delete alias ".concat(alias.name),
                                    "data-testid": "alias-delete-".concat(alias.name),
                                    children: "Delete"
                                })
                            ]
                        })
                    ]
                }, alias.name);
            }),
            showForm && /*#__PURE__*/ (0,jsx_runtime.jsx)("section", {
                "aria-labelledby": "alias-form-title",
                children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                    className: AliasesManager_css_formCard,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                            id: "alias-form-title",
                            className: AliasesManager_css_formTitle,
                            children: editingName ? "Edit Alias: ".concat(editingName) : "New Alias"
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: AliasesManager_css_formFields,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: AliasesManager_css_field,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                            className: AliasesManager_css_label,
                                            htmlFor: "alias-name",
                                            children: "Name *"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                            id: "alias-name",
                                            ref: nameInputRef,
                                            type: "text",
                                            className: AliasesManager_css_input,
                                            placeholder: "e.g. myproject",
                                            value: form.name,
                                            onChange: (e)=>{
                                                setForm({
                                                    ...form,
                                                    name: e.target.value
                                                });
                                                setNameError(null);
                                            },
                                            disabled: !!editingName,
                                            "aria-required": "true"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                            className: previewHint,
                                            children: [
                                                "Preview: @",
                                                form.name || "name"
                                            ]
                                        }),
                                        nameError && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                            className: AliasesManager_css_fieldError,
                                            role: "alert",
                                            children: nameError
                                        })
                                    ]
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: AliasesManager_css_field,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                            className: AliasesManager_css_label,
                                            htmlFor: "alias-desc",
                                            children: "Description"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                            id: "alias-desc",
                                            type: "text",
                                            className: AliasesManager_css_input,
                                            placeholder: "Short description",
                                            value: form.description,
                                            onChange: (e)=>setForm({
                                                    ...form,
                                                    description: e.target.value
                                                })
                                        })
                                    ]
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: AliasesManager_css_field,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                            className: AliasesManager_css_label,
                                            htmlFor: "alias-group",
                                            children: "Group"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                            id: "alias-group",
                                            type: "text",
                                            className: AliasesManager_css_input,
                                            placeholder: "e.g. work, personal",
                                            value: form.group,
                                            onChange: (e)=>setForm({
                                                    ...form,
                                                    group: e.target.value
                                                })
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                            className: groupHint,
                                            children: "Used to organize aliases in the palette."
                                        })
                                    ]
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: AliasesManager_css_field,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                            className: AliasesManager_css_label,
                                            htmlFor: "alias-path",
                                            children: "Path"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)(RepoPathInput/* RepoPathInput */.E, {
                                            id: "alias-path",
                                            placeholder: "e.g. ~/code/myproject",
                                            value: form.path,
                                            onChange: (v)=>setForm({
                                                    ...form,
                                                    path: v
                                                })
                                        })
                                    ]
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: AliasesManager_css_field,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                            className: AliasesManager_css_label,
                                            htmlFor: "alias-profile",
                                            children: "Profile"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                            id: "alias-profile",
                                            type: "text",
                                            className: AliasesManager_css_input,
                                            placeholder: "e.g. fast-mode",
                                            value: form.profile,
                                            onChange: (e)=>setForm({
                                                    ...form,
                                                    profile: e.target.value
                                                })
                                        })
                                    ]
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: AliasesManager_css_field,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                            className: AliasesManager_css_label,
                                            htmlFor: "alias-program",
                                            children: "Program"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("select", {
                                            id: "alias-program",
                                            className: AliasesManager_css_select,
                                            value: form.program,
                                            onChange: (e)=>setForm({
                                                    ...form,
                                                    program: e.target.value
                                                }),
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                                    value: "",
                                                    children: "Default"
                                                }),
                                                programs/* PROGRAMS */.i.map((p)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                                        value: p.value,
                                                        children: p.label
                                                    }, p.value))
                                            ]
                                        })
                                    ]
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: AliasesManager_css_field,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                            className: AliasesManager_css_label,
                                            htmlFor: "alias-session-type",
                                            children: "Session Type"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("select", {
                                            id: "alias-session-type",
                                            className: AliasesManager_css_select,
                                            value: form.sessionType,
                                            onChange: (e)=>setForm({
                                                    ...form,
                                                    sessionType: Number(e.target.value)
                                                }),
                                            children: SESSION_TYPE_OPTIONS.map((opt)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("option", {
                                                    value: opt.value,
                                                    children: opt.label
                                                }, opt.value))
                                        })
                                    ]
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: AliasesManager_css_field,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                            className: AliasesManager_css_label,
                                            htmlFor: "alias-name-prefix",
                                            children: "Name prefix"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                            id: "alias-name-prefix",
                                            className: AliasesManager_css_input,
                                            type: "text",
                                            placeholder: "e.g. ssq-",
                                            value: form.namePrefix,
                                            onChange: (e)=>setForm({
                                                    ...form,
                                                    namePrefix: e.target.value
                                                })
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                            className: groupHint,
                                            children: [
                                                "Prepended to the label you type after the alias — e.g.",
                                                " ",
                                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("code", {
                                                    children: [
                                                        "@",
                                                        form.name || "alias",
                                                        " my-feature"
                                                    ]
                                                }),
                                                " → session name",
                                                " ",
                                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("code", {
                                                    children: [
                                                        form.namePrefix || "(prefix)",
                                                        "my-feature"
                                                    ]
                                                })
                                            ]
                                        })
                                    ]
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                    className: AliasesManager_css_field,
                                    children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                        className: AliasesManager_css_checkboxLabel,
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                type: "checkbox",
                                                checked: form.autoYes,
                                                onChange: (e)=>setForm({
                                                        ...form,
                                                        autoYes: e.target.checked
                                                    })
                                            }),
                                            "Auto-yes"
                                        ]
                                    })
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: AliasesManager_css_field,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                            className: AliasesManager_css_label,
                                            children: "Tags"
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                            className: AliasesManager_css_tagList,
                                            children: form.tags.map((t)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                    className: AliasesManager_css_tag,
                                                    children: [
                                                        t,
                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                            type: "button",
                                                            className: AliasesManager_css_tagRemove,
                                                            onClick: ()=>handleRemoveTag(t),
                                                            "aria-label": "Remove tag ".concat(t),
                                                            children: "x"
                                                        })
                                                    ]
                                                }, t))
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                            className: AliasesManager_css_tagInputRow,
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                    type: "text",
                                                    className: AliasesManager_css_input,
                                                    placeholder: "Add a tag...",
                                                    value: form.tagInput,
                                                    onChange: (e)=>setForm({
                                                            ...form,
                                                            tagInput: e.target.value
                                                        }),
                                                    onKeyDown: handleTagKeyDown
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                    type: "button",
                                                    className: "btn btn-secondary",
                                                    onClick: handleAddTag,
                                                    children: "Add"
                                                })
                                            ]
                                        })
                                    ]
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                    className: AliasesManager_css_field,
                                    children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                                        className: AliasesManager_css_checkboxLabel,
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                type: "checkbox",
                                                checked: form.showAdvanced,
                                                onChange: (e)=>setForm({
                                                        ...form,
                                                        showAdvanced: e.target.checked
                                                    })
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: advancedToggle,
                                                children: "Advanced"
                                            })
                                        ]
                                    })
                                }),
                                form.showAdvanced && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                            className: AliasesManager_css_field,
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                                    className: AliasesManager_css_label,
                                                    children: "Environment Variables"
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                                    className: AliasesManager_css_envVarTable,
                                                    children: form.envVars.map((ev, i)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                            className: AliasesManager_css_envVarRow,
                                                            children: [
                                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                                    type: "text",
                                                                    className: envVarInput,
                                                                    placeholder: "KEY",
                                                                    value: ev.key,
                                                                    onChange: (e)=>handleEnvVarChange(i, "key", e.target.value),
                                                                    "aria-label": "Environment variable key ".concat(i + 1)
                                                                }),
                                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                                    type: "text",
                                                                    className: envVarInput,
                                                                    placeholder: "value",
                                                                    value: ev.value,
                                                                    onChange: (e)=>handleEnvVarChange(i, "value", e.target.value),
                                                                    "aria-label": "Environment variable value ".concat(i + 1)
                                                                }),
                                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                                    type: "button",
                                                                    className: AliasesManager_css_deleteBtn,
                                                                    onClick: ()=>handleRemoveEnvVar(i),
                                                                    "aria-label": "Remove environment variable ".concat(ev.key || String(i + 1)),
                                                                    children: "Remove"
                                                                })
                                                            ]
                                                        }, i))
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                    type: "button",
                                                    className: "btn btn-secondary",
                                                    onClick: handleAddEnvVar,
                                                    children: "Add variable"
                                                })
                                            ]
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                            className: AliasesManager_css_field,
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                                    className: AliasesManager_css_label,
                                                    htmlFor: "alias-cliflags",
                                                    children: "CLI Flags"
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                    id: "alias-cliflags",
                                                    type: "text",
                                                    className: AliasesManager_css_input,
                                                    placeholder: "e.g. --verbose",
                                                    value: form.cliFlags,
                                                    onChange: (e)=>setForm({
                                                            ...form,
                                                            cliFlags: e.target.value
                                                        })
                                                })
                                            ]
                                        })
                                    ]
                                })
                            ]
                        }),
                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: AliasesManager_css_formActions,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    type: "button",
                                    className: "btn btn-primary",
                                    onClick: handleSave,
                                    disabled: saving,
                                    children: saving ? "Saving..." : "Save"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                    type: "button",
                                    className: "btn btn-secondary",
                                    onClick: handleCancel,
                                    children: "Cancel"
                                })
                            ]
                        })
                    ]
                })
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/lib/api/transport.ts
var transport = __webpack_require__(21694);
// EXTERNAL MODULE: ./src/lib/hooks/useProbeProgram.ts
var useProbeProgram = __webpack_require__(62182);
// EXTERNAL MODULE: ./src/components/ui/ProbeStatusBadge.tsx + 1 modules
var ProbeStatusBadge = __webpack_require__(6990);
;// ./src/lib/flags/flagTokens.ts
/** Structural subset of the proto FlagInfo; the generated type satisfies it. */ const isSpace = (ch)=>/\s/.test(ch);
/**
 * The whitespace-delimited token around `caret`, or null unless it starts with
 * `-`, is not the `--` terminator, and is not inside quotes or after `--`.
 */ function tokenAtCaret(value, caret) {
    const c = Math.max(0, Math.min(caret, value.length));
    let start = c;
    while(start > 0 && !isSpace(value[start - 1]))start--;
    let end = c;
    while(end < value.length && !isSpace(value[end]))end++;
    const text = value.slice(start, end);
    if (!text.startsWith("-") || text === "--") return null;
    const before = value.slice(0, start);
    if (before.split(/\s+/).includes("--")) return null;
    var _before_match;
    const quotes = ((_before_match = before.match(/["']/g)) !== null && _before_match !== void 0 ? _before_match : []).length;
    if (quotes % 2 === 1) return null;
    return {
        start,
        end,
        text
    };
}
/** Replaces `token` with `flag.name` plus one trailing space (never `=`). */ function applyCompletion(value, token, flag) {
    const rest = value.slice(token.end);
    const insert = flag.name + (rest.startsWith(" ") ? "" : " ");
    const head = value.slice(0, token.start) + insert;
    return {
        value: head + rest,
        caret: head.length + (rest.startsWith(" ") ? 1 : 0)
    };
}
/** Prefix matches first, then substring matches, over name, short and aliases. */ function filterFlags(flags, text) {
    const q = text.toLowerCase();
    if (!q) return [];
    const names = (f)=>[
            f.name,
            f.short,
            ...f.aliases
        ].filter(Boolean).map((n)=>n.toLowerCase());
    const prefix = [];
    const substring = [];
    for (const f of flags){
        const ns = names(f);
        if (ns.some((n)=>n.startsWith(q))) prefix.push(f);
        else if (ns.some((n)=>n.includes(q))) substring.push(f);
    }
    return [
        ...prefix,
        ...substring
    ];
}

;// ./src/lib/flags/useListboxNav.ts

const CLOSED = {
    open: false,
    active: -1
};
/** Pure key-to-state reducer shared by combobox-style listboxes. */ function listboxKey(state, key, altKey, count) {
    const unhandled = {
        state,
        accept: null,
        handled: false
    };
    if (count === 0) return unhandled;
    const hasActive = state.open && state.active >= 0 && state.active < count;
    switch(key){
        case "ArrowDown":
            if (altKey) return {
                state: {
                    open: true,
                    active: state.active
                },
                accept: null,
                handled: true
            };
            return {
                state: {
                    open: true,
                    active: state.open ? (state.active + 1) % count : 0
                },
                accept: null,
                handled: true
            };
        case "ArrowUp":
            {
                const active = !state.open || state.active <= 0 ? count - 1 : state.active - 1;
                return {
                    state: {
                        open: true,
                        active
                    },
                    accept: null,
                    handled: true
                };
            }
        case "Escape":
            return state.open ? {
                state: CLOSED,
                accept: null,
                handled: true
            } : unhandled;
        case "Enter":
            return hasActive ? {
                state: CLOSED,
                accept: state.active,
                handled: true
            } : unhandled;
        case "Tab":
            if (hasActive) return {
                state: CLOSED,
                accept: state.active,
                handled: true
            };
            return state.open ? {
                state: CLOSED,
                accept: null,
                handled: false
            } : unhandled;
        default:
            return unhandled;
    }
}
function useListboxNav() {
    const [state, setState] = (0,react.useState)(CLOSED);
    const onKey = (0,react.useCallback)((key, altKey, count)=>{
        const result = listboxKey(state, key, altKey, count);
        if (result.state !== state) setState(result.state);
        return result;
    }, [
        state
    ]);
    const open = (0,react.useCallback)(()=>setState({
            open: true,
            active: -1
        }), []);
    const close = (0,react.useCallback)(()=>setState(CLOSED), []);
    return {
        ...state,
        onKey,
        openList: open,
        close
    };
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fui%2FFlagCombobox.css.ts.vanilla.css%22%2C%22source%22%3A%22Ll8xNzZhcHVnMCB7CiAgbGlzdC1zdHlsZTogbm9uZTsKICBtYXJnaW46IHZhcigtLV8xOWV6MnJ6MnkpIDAgMDsKICBwYWRkaW5nOiAwOwogIG1heC1oZWlnaHQ6IG1pbig0MHZoLCAzNTJweCk7CiAgb3ZlcmZsb3cteTogYXV0bzsKICBiYWNrZ3JvdW5kLWNvbG9yOiB2YXIoLS1fMTllejJyejFhKTsKICBib3JkZXI6IDFweCBzb2xpZCB2YXIoLS1fMTllejJyemspOwogIGJvcmRlci1yYWRpdXM6IDRweDsKfQouXzE3NmFwdWcxIHsKICBtaW4taGVpZ2h0OiA0NHB4OwogIHBhZGRpbmc6IHZhcigtLV8xOWV6MnJ6MnopIHZhcigtLV8xOWV6MnJ6MzApOwogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBqdXN0aWZ5LWNvbnRlbnQ6IGNlbnRlcjsKICBjdXJzb3I6IHBvaW50ZXI7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MWIpOwogIGZvbnQtc2l6ZTogMC44NzVyZW07Cn0KLl8xNzZhcHVnMVthcmlhLXNlbGVjdGVkPSJ0cnVlIl0gewogIGJhY2tncm91bmQtY29sb3I6IHZhcigtLV8xOWV6MnJ6OCk7Cn0KLl8xNzZhcHVnMiB7CiAgZm9udC1mYW1pbHk6IG1vbm9zcGFjZTsKfQouXzE3NmFwdWczIHsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoxKTsKfQouXzE3NmFwdWc0IHsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoxKTsKICBmb250LXNpemU6IDAuODEyNXJlbTsKfQ%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fui_2FFlagCombobox_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll8xNzZhcHVnMCB7CiAgbGlzdC1zdHlsZTogbm9uZTsKICBtYXJnaW46IHZhcigtLV8xOWV6MnJ6MnkpIDAgMDsKICBwYWRkaW5nOiAwOwogIG1heC1oZWlnaHQ6IG1pbig0MHZoLCAzNTJweCk7CiAgb3ZlcmZsb3cteTogYXV0bzsKICBiYWNrZ3JvdW5kLWNvbG9yOiB2YXIoLS1fMTllejJyejFhKTsKICBib3JkZXI6IDFweCBzb2xpZCB2YXIoLS1fMTllejJyemspOwogIGJvcmRlci1yYWRpdXM6IDRweDsKfQouXzE3NmFwdWcxIHsKICBtaW4taGVpZ2h0OiA0NHB4OwogIHBhZGRpbmc6IHZhcigtLV8xOWV6MnJ6MnopIHZhcigtLV8xOWV6MnJ6MzApOwogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBqdXN0aWZ5LWNvbnRlbnQ6IGNlbnRlcjsKICBjdXJzb3I6IHBvaW50ZXI7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MWIpOwogIGZvbnQtc2l6ZTogMC44NzVyZW07Cn0KLl8xNzZhcHVnMVthcmlhLXNlbGVjdGVkPSJ0cnVlIl0gewogIGJhY2tncm91bmQtY29sb3I6IHZhcigtLV8xOWV6MnJ6OCk7Cn0KLl8xNzZhcHVnMiB7CiAgZm9udC1mYW1pbHk6IG1vbm9zcGFjZTsKfQouXzE3NmFwdWczIHsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoxKTsKfQouXzE3NmFwdWc0IHsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoxKTsKICBmb250LXNpemU6IDAuODEyNXJlbTsKfQ_22_7D = __webpack_require__(48608);
;// ./src/components/ui/FlagCombobox.css.ts


var listbox = '_176apug0';
var FlagCombobox_css_option = '_176apug1';
var optionDescription = '_176apug4';
var optionName = '_176apug2';
var optionValue = '_176apug3';
;// ./src/components/ui/FlagCombobox.tsx
// +feature: settings-programs
/* __next_internal_client_entry_do_not_use__ FlagCombobox auto */ 




/**
 * Text input that completes the flag token at the caret. Always renders the
 * same <input>, so focus and caret survive flags arriving after a late probe.
 */ function FlagCombobox(param) {
    let { id, value, onChange, flags, testId, describedBy, className, placeholder } = param;
    const inputRef = (0,react.useRef)(null);
    const pendingCaret = (0,react.useRef)(null);
    const [caret, setCaret] = (0,react.useState)(value.length);
    const nav = useListboxNav();
    const hasFlags = flags.length > 0;
    const token = tokenAtCaret(value, caret);
    const matches = hasFlags && token ? filterFlags(flags, token.text) : [];
    const expanded = nav.open && matches.length > 0;
    const listboxId = "".concat(id, "-listbox");
    const optionId = (i)=>"".concat(id, "-option-").concat(i);
    const activeFlag = expanded && nav.active >= 0 ? matches[nav.active] : undefined;
    const activeDescId = (activeFlag === null || activeFlag === void 0 ? void 0 : activeFlag.description) ? "".concat(id, "-active-description") : undefined;
    const describedByIds = [
        activeDescId,
        describedBy
    ].filter(Boolean).join(" ") || undefined;
    (0,react.useLayoutEffect)(()=>{
        var _inputRef_current;
        if (pendingCaret.current === null) return;
        (_inputRef_current = inputRef.current) === null || _inputRef_current === void 0 ? void 0 : _inputRef_current.setSelectionRange(pendingCaret.current, pendingCaret.current);
        pendingCaret.current = null;
    });
    (0,react.useEffect)(()=>{
        var _document_getElementById_scrollIntoView, _document_getElementById;
        if (!expanded || nav.active < 0) return;
        (_document_getElementById = document.getElementById(optionId(nav.active))) === null || _document_getElementById === void 0 ? void 0 : (_document_getElementById_scrollIntoView = _document_getElementById.scrollIntoView) === null || _document_getElementById_scrollIntoView === void 0 ? void 0 : _document_getElementById_scrollIntoView.call(_document_getElementById, {
            block: "nearest"
        });
    // optionId is derived from id
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        expanded,
        nav.active
    ]);
    const accept = (index)=>{
        if (!token) return;
        const next = applyCompletion(value, token, matches[index]);
        pendingCaret.current = next.caret;
        setCaret(next.caret);
        onChange(next.value);
    };
    const syncCaret = (el)=>{
        var _el_selectionStart;
        return setCaret((_el_selectionStart = el.selectionStart) !== null && _el_selectionStart !== void 0 ? _el_selectionStart : el.value.length);
    };
    const handleKeyDown = (e)=>{
        const result = nav.onKey(e.key, e.altKey, matches.length);
        if (result.handled) e.preventDefault();
        if (result.accept !== null) accept(result.accept);
    };
    const comboboxProps = hasFlags ? {
        role: "combobox",
        "aria-autocomplete": "list",
        "aria-expanded": expanded,
        "aria-controls": expanded ? listboxId : undefined,
        "aria-activedescendant": expanded && nav.active >= 0 ? optionId(nav.active) : undefined
    } : {};
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                ref: inputRef,
                id: id,
                type: "text",
                className: className,
                value: value,
                placeholder: placeholder,
                onChange: (e)=>{
                    syncCaret(e.target);
                    onChange(e.target.value);
                    nav.openList();
                },
                onSelect: (e)=>syncCaret(e.currentTarget),
                onKeyDown: handleKeyDown,
                onBlur: nav.close,
                autoCapitalize: "off",
                autoCorrect: "off",
                autoComplete: "off",
                spellCheck: false,
                "aria-describedby": describedByIds,
                "data-testid": testId,
                ...comboboxProps
            }),
            expanded && /*#__PURE__*/ (0,jsx_runtime.jsx)("ul", {
                id: listboxId,
                role: "listbox",
                "aria-label": "Flag suggestions",
                className: listbox,
                children: matches.map((flag, i)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("li", {
                        id: optionId(i),
                        role: "option",
                        "aria-selected": i === nav.active,
                        "aria-label": flag.takesValue ? "".concat(flag.name, ", takes a value") : flag.name,
                        className: FlagCombobox_css_option,
                        // Keeps focus in the input so selecting never blurs (and re-probes).
                        onMouseDown: (e)=>{
                            e.preventDefault();
                            accept(i);
                            nav.close();
                        },
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                className: optionName,
                                children: [
                                    flag.name,
                                    flag.takesValue && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: optionValue,
                                        children: " <value>"
                                    })
                                ]
                            }),
                            i === nav.active && flag.description && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                id: activeDescId,
                                className: optionDescription,
                                children: flag.description
                            })
                        ]
                    }, flag.name))
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/components/ui/UnknownFlagsWarning.tsx
var UnknownFlagsWarning = __webpack_require__(391);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fui%2FFlagInfoButton.css.ts.vanilla.css%22%2C%22source%22%3A%22Ll8xMmdqYXlxMCB7CiAgbWluLXdpZHRoOiA0NHB4OwogIG1pbi1oZWlnaHQ6IDQ0cHg7CiAgZGlzcGxheTogaW5saW5lLWZsZXg7CiAgYWxpZ24taXRlbXM6IGNlbnRlcjsKICBqdXN0aWZ5LWNvbnRlbnQ6IGNlbnRlcjsKICBwYWRkaW5nOiAwOwogIGJhY2tncm91bmQ6IHRyYW5zcGFyZW50OwogIGJvcmRlcjogbm9uZTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoxKTsKICBjdXJzb3I6IHBvaW50ZXI7Cn0KLl8xMmdqYXlxMFthcmlhLWV4cGFuZGVkPSJ0cnVlIl0gewogIGNvbG9yOiB2YXIoLS1fMTllejJyejApOwp9Ci5fMTJnamF5cTEgewogIGNvbG9yOiB2YXIoLS1fMTllejJyejEpOwogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYik7CiAgb3ZlcmZsb3ctd3JhcDogYW55d2hlcmU7CiAgZmxleC1iYXNpczogMTAwJTsKfQ%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fui_2FFlagInfoButton_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll8xMmdqYXlxMCB7CiAgbWluLXdpZHRoOiA0NHB4OwogIG1pbi1oZWlnaHQ6IDQ0cHg7CiAgZGlzcGxheTogaW5saW5lLWZsZXg7CiAgYWxpZ24taXRlbXM6IGNlbnRlcjsKICBqdXN0aWZ5LWNvbnRlbnQ6IGNlbnRlcjsKICBwYWRkaW5nOiAwOwogIGJhY2tncm91bmQ6IHRyYW5zcGFyZW50OwogIGJvcmRlcjogbm9uZTsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoxKTsKICBjdXJzb3I6IHBvaW50ZXI7Cn0KLl8xMmdqYXlxMFthcmlhLWV4cGFuZGVkPSJ0cnVlIl0gewogIGNvbG9yOiB2YXIoLS1fMTllejJyejApOwp9Ci5fMTJnamF5cTEgewogIGNvbG9yOiB2YXIoLS1fMTllejJyejEpOwogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYik7CiAgb3ZlcmZsb3ctd3JhcDogYW55d2hlcmU7CiAgZmxleC1iYXNpczogMTAwJTsKfQ_22_7D = __webpack_require__(66280);
;// ./src/components/ui/FlagInfoButton.css.ts


var FlagInfoButton_css_button = '_12gjayq0';
var FlagInfoButton_css_description = '_12gjayq1';
;// ./src/components/ui/FlagInfoButton.tsx
// +feature: settings-programs
/* __next_internal_client_entry_do_not_use__ FlagInfoButton auto */ 



/**
 * Tap-to-toggle inline description. No hover handler: every device reads it the
 * same way. Renders nothing for a flag without a description. Must stay outside
 * role=option (nested interactive content).
 */ function FlagInfoButton(param) {
    let { flag, description } = param;
    const [open, setOpen] = (0,react.useState)(false);
    const descriptionId = (0,react.useId)();
    if (!description) return null;
    const toggle = // analytics-exempt
    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
        type: "button",
        className: FlagInfoButton_css_button,
        "aria-expanded": open,
        "aria-controls": descriptionId,
        "aria-label": "".concat(open ? "Hide" : "Show", " description for ").concat(flag),
        onClick: ()=>setOpen((v)=>!v),
        "data-testid": "prog-flag-info-button",
        children: /*#__PURE__*/ (0,jsx_runtime.jsx)(ProbeStatusBadge/* Icon */.In, {
            name: "info"
        })
    });
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
        children: [
            toggle,
            open && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                id: descriptionId,
                className: FlagInfoButton_css_description,
                "data-testid": "prog-flag-info-description",
                children: description
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/components/ui/FlagNotes.css.ts
var FlagNotes_css = __webpack_require__(31750);
;// ./src/components/ui/AvailableFlags.tsx
// +feature: settings-programs
/* __next_internal_client_entry_do_not_use__ AvailableFlags auto */ 



/** "Available flags (N)" disclosure; descriptions open via FlagInfoButton, outside any listbox. */ function AvailableFlags(param) {
    let { flags, testId } = param;
    const [open, setOpen] = (0,react.useState)(false);
    const listId = (0,react.useId)();
    if (flags.length === 0) return null;
    const toggle = // analytics-exempt
    /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
        type: "button",
        className: FlagNotes_css/* disclosureToggle */.I3,
        "aria-expanded": open,
        "aria-controls": listId,
        onClick: ()=>setOpen((v)=>!v),
        "data-testid": "".concat(testId, "-toggle"),
        children: [
            "Available flags (",
            flags.length,
            ") ",
            open ? "▴" : "▾"
        ]
    });
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        "data-testid": testId,
        children: [
            toggle,
            open && /*#__PURE__*/ (0,jsx_runtime.jsx)("ul", {
                id: listId,
                className: FlagNotes_css/* flagList */.zg,
                children: flags.map((f)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("li", {
                        className: FlagNotes_css/* flagItem */.KQ,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                className: FlagNotes_css/* flagName */.GI,
                                children: [
                                    f.name,
                                    f.takesValue ? " <value>" : ""
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)(FlagInfoButton, {
                                flag: f.name,
                                description: f.description
                            })
                        ]
                    }, f.name))
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/lib/flags/validateFlags.ts
var validateFlags = __webpack_require__(22017);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsettings%2FProgramsManager.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA81XS5OjIBC-51dwnBxMiXnNkMv-ky1UoiQKLuDksZX_viU6CSIaJ5OZ2sohVQpNf4_uxtnuPVW-9MHfCQAxlUWGTwhsM3LcTID-92IqSKQoZwhEPCtzVr1JcIEAFCTfTC6TWR0E6iARz7hA4B2LF8_7Dd_IORBnf6rDcaY8Sc8EATgLlnp38_RAaJIqBFa-Xz3LsUgoQ8A3wgfuHHelVHR78iLOFGEKAVngiHghUQdCdK44ownzqCK5RCAiTBFhhJ33Zx1MjXWL4XUtdP7sdb1sk7N8SvZaEKmwUJYIABQ4jilLqsPXH9SGONongpcs9pyJr3XiIRcxEQjA4ggkz2hsrSLGKk_gmJYSgVVxNNCtHvKPfzNBTpl3oLFK25qv3XGdil6DWsy_jrKlZcCWmG9zW803HdOgHGokwJ_Nb-S3CVsUx07gtbMGlnUK98SDtXoDwO4JuzX9jf8DSEHaDymgozAFOxNU2K89dBQtDCydo08Vve2S2O3exqY3WuuqTgVl-7b9SVsT-NNlvf1MRx_TzqtfTVMLZ_Jg-7AJTx-M06lv-phvXOivMXd2gTXlNbpbQzxK171L16ZU3XHD_vlVk4cANIGgLY9KqeHwUmWUEQQYZ8Q413lQZpbmfrA4lu2p5inuFDx7SHCrkM9mWvlAWo59924XTIe7cvi9IreXzFf9zbSR_GpGC9lp6sZqWMQ6KzQ5ZE8xCNf77_IFR_EF_S8SVrf7qBSyellw-nHpeB6HuHsbsVYkJj_FOH7mLX6CPn7mX-Nn2UfPEBz6ZBNik54_Ay28M7ycStRTq249ndyMk8TTm1BdOtWdOG2YWzQN3Lgnfzy6TRQbRlMaHYcogZkssCBMfctAudGLAGUpEVQ53XHDi2IqcZiR-sZ0XSh4IoiU1WZe4Iiqkx4Cxk71iUHtdMuvnMQUgxeD2NXCL45THbilcFdSwQ8b_WLgI-0yufwDVn7kS2UPAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsettings_2FProgramsManager_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA81XS5OjIBC_51dwnBxMiXnNkMv_ky1UoiQKLuDksZX_viU6CSIaJ5OZ2sohVQpNf4_uxtnuPVW_9MHfCQAxlUWGTwhsM3LcTID_92IqSKQoZwhEPCtzVr1JcIEAFCTfTC6TWR0E6iARz7hA4B2LF8_7Dd_IORBnf6rDcaY8Sc8EATgLlnp38_RAaJIqBFa_Xz3LsUgoQ8A3wgfuHHelVHR78iLOFGEKAVngiHghUQdCdK44ownzqCK5RCAiTBFhhJ33Zx1MjXWL4XUtdP7sdb1sk7N8SvZaEKmwUJYIABQ4jilLqsPXH9SGONongpcs9pyJr3XiIRcxEQjA4ggkz2hsrSLGKk_gmJYSgVVxNNCtHvKPfzNBTpl3oLFK25qv3XGdil6DWsy_jrKlZcCWmG9zW803HdOgHGokwJ_Nb_S3CVsUx07gtbMGlnUK98SDtXoDwO4JuzX9jf8DSEHaDymgozAFOxNU2K89dBQtDCydo08Vve2S2O3exqY3WuuqTgVl_7b9SVsT_NNlvf1MRx_TzqtfTVMLZ_Jg_7AJTx_M06lv_phvXOivMXd2gTXlNbpbQzxK171L16ZU3XHD_vlVk4cANIGgLY9KqeHwUmWUEQQYZ8Q413lQZpbmfrA4lu2p5inuFDx7SHCrkM9mWvlAWo59924XTIe7cvi9IreXzFf9zbSR_GpGC9lp6sZqWMQ6KzQ5ZE8xCNf77_IFR_EF_S8SVrf7qBSyellw_nHpeB6HuHsbsVYkJj_FOH7mLX6CPn7mX_Nn2UfPEBz6ZBNik54_Ay28M7ycStRTq249ndyMk8TTm1BdOtWdOG2YWzQN3Lgnfzy6TRQbRlMaHYcogZkssCBMfctAudGLAGUpEVQ53XHDi2IqcZiR_sZ0XSh4IoiU1WZe4Iiqkx4Cxk71iUHtdMuvnMQUgxeD2NXCL45THbilcFdSwQ8b_WLgI_0yufwDVn7kS2UPAAA_22_7D = __webpack_require__(17077);
;// ./src/components/settings/ProgramsManager.css.ts


var badgeBuiltin = 'jvht0s9';
var badgeCustom = 'jvht0sa';
var checkButton = 'jvht0ss';
var commandRow = 'jvht0sr';
var ProgramsManager_css_confirmDeleteBtn = 'jvht0sp';
var ProgramsManager_css_container = 'jvht0s0';
var ProgramsManager_css_deleteBtn = 'jvht0so';
var ProgramsManager_css_emptyText = 'jvht0s4';
var ProgramsManager_css_envVarInput = 'jvht0sn';
var ProgramsManager_css_envVarRow = 'jvht0sm';
var ProgramsManager_css_envVarTable = 'jvht0sl';
var ProgramsManager_css_field = 'jvht0sh';
var ProgramsManager_css_fieldError = 'jvht0sq';
var ProgramsManager_css_formActions = 'jvht0sk';
var ProgramsManager_css_formCard = 'jvht0se';
var ProgramsManager_css_formFields = 'jvht0sg';
var ProgramsManager_css_formTitle = 'jvht0sf';
var ProgramsManager_css_headerRow = 'jvht0s2';
var ProgramsManager_css_heading = 'jvht0s1';
var hintText = 'jvht0st';
var ProgramsManager_css_input = 'jvht0sj';
var ProgramsManager_css_label = 'jvht0si';
var ProgramsManager_css_loadingText = 'jvht0s3';
var programActions = 'jvht0sd';
var programDesc = 'jvht0sb';
var programInfo = 'jvht0s6';
var programLabel = 'jvht0s8';
var programMeta = 'jvht0sc';
var programRow = 'jvht0s5';
var programTitleRow = 'jvht0s7';
;// ./src/components/settings/ProgramsManager.tsx
/* __next_internal_client_entry_do_not_use__ ProgramsManager auto */ 
// analytics-exempt
// +feature: settings-programs











// Explains the absence of suggestions; states not listed need no hint.
const FLAGS_HINTS = {
    idle: "Check the command above to enable flag suggestions",
    needsConfirm: "Suggestions appear after Check reads the flags."
};
const PROGRAM_ID_RE = /^[\w-]+$/;
const ProgramsManager_emptyForm = {
    id: "",
    label: "",
    command: "",
    cliFlags: "",
    description: "",
    envVars: []
};
function ProgramsManager() {
    const [programs, setPrograms] = (0,react.useState)([]);
    const [loading, setLoading] = (0,react.useState)(true);
    const [error, setError] = (0,react.useState)(null);
    const [idError, setIdError] = (0,react.useState)(null);
    const [success, setSuccess] = (0,react.useState)(null);
    const [showForm, setShowForm] = (0,react.useState)(false);
    const [isEditing, setIsEditing] = (0,react.useState)(false);
    const [formData, setFormData] = (0,react.useState)(ProgramsManager_emptyForm);
    const [deletingId, setDeletingId] = (0,react.useState)(null);
    const [confirmDeleteId, setConfirmDeleteId] = (0,react.useState)(null);
    const probe = (0,useProbeProgram/* useProbeProgram */.eT)(formData.command, "program-config");
    const flagsHint = FLAGS_HINTS[probe.state.kind];
    // A disabled button drops focus in browsers, so hand it back once the run settles.
    const checkButtonRef = (0,react.useRef)(null);
    const checkClicked = (0,react.useRef)(false);
    const wasChecking = (0,react.useRef)(false);
    (0,react.useEffect)(()=>{
        const checking = probe.state.kind === "checking";
        if (wasChecking.current && !checking && checkClicked.current && document.activeElement === document.body) {
            var _checkButtonRef_current;
            (_checkButtonRef_current = checkButtonRef.current) === null || _checkButtonRef_current === void 0 ? void 0 : _checkButtonRef_current.focus();
        }
        if (!checking) checkClicked.current = false;
        wasChecking.current = checking;
    }, [
        probe.state.kind
    ]);
    const [flagsFocused, setFlagsFocused] = (0,react.useState)(false);
    const probedFlags = (0,react.useMemo)(()=>probe.state.kind === "found" ? probe.state.flags : [], [
        probe.state
    ]);
    // The token still being typed is not judged until the field blurs or a space follows it.
    const settledFlags = flagsFocused && !/\s$/.test(formData.cliFlags) ? formData.cliFlags.replace(/\S+$/, "") : formData.cliFlags;
    const unknownFlags = (0,react.useMemo)(()=>(0,validateFlags/* validateFlags */.k)(settledFlags, probedFlags), [
        settledFlags,
        probedFlags
    ]);
    const flagsDescribedBy = [
        flagsHint && "prog-flags-hint",
        unknownFlags.length > 0 && "prog-flags-warning"
    ].filter(Boolean).join(" ") || undefined;
    const getClient = (0,react.useCallback)(()=>{
        return (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, (0,transport/* getConnectTransport */.qW)());
    }, []);
    const fetchPrograms = (0,react.useCallback)(async ()=>{
        setLoading(true);
        setError(null);
        try {
            const client = getClient();
            const res = await client.listProgramsConfig({});
            var _res_programs;
            setPrograms((_res_programs = res.programs) !== null && _res_programs !== void 0 ? _res_programs : []);
        } catch (err) {
            const msg = err instanceof Error ? err.message : String(err);
            setError("Failed to load programs: ".concat(msg));
        } finally{
            setLoading(false);
        }
    }, [
        getClient
    ]);
    (0,react.useEffect)(()=>{
        fetchPrograms();
    }, [
        fetchPrograms
    ]);
    const handleOpenCreate = ()=>{
        setFormData(ProgramsManager_emptyForm);
        setIsEditing(false);
        setIdError(null);
        setError(null);
        setShowForm(true);
    };
    const handleOpenEdit = (p)=>{
        const envVars = p.env ? Object.entries(p.env).map((param)=>{
            let [key, value] = param;
            return {
                key,
                value
            };
        }) : [];
        setFormData({
            id: p.id,
            label: p.label,
            command: p.command,
            cliFlags: p.cliFlags,
            description: p.description,
            envVars
        });
        setIsEditing(true);
        setIdError(null);
        setError(null);
        setShowForm(true);
    };
    const handleDuplicate = (p)=>{
        const envVars = p.env ? Object.entries(p.env).map((param)=>{
            let [key, value] = param;
            return {
                key,
                value
            };
        }) : [];
        setFormData({
            id: "".concat(p.id, "-copy"),
            label: "".concat(p.label, " (Copy)"),
            command: p.command,
            cliFlags: p.cliFlags,
            description: p.description,
            envVars
        });
        setIsEditing(false);
        setIdError(null);
        setError(null);
        setShowForm(true);
    };
    const handleCancelForm = ()=>{
        setShowForm(false);
        setFormData(ProgramsManager_emptyForm);
        setIdError(null);
    };
    const validateForm = ()=>{
        let valid = true;
        setIdError(null);
        const trimmedId = formData.id.trim();
        if (!trimmedId) {
            setIdError("Program ID is required.");
            valid = false;
        } else if (!PROGRAM_ID_RE.test(trimmedId)) {
            setIdError("Program ID can only contain letters, digits, hyphens, and underscores.");
            valid = false;
        } else {
            const isBuiltin = programs.some((p)=>p.isBuiltin && p.id.toLowerCase() === trimmedId.toLowerCase());
            if (isBuiltin) {
                setIdError("Cannot override a built-in program ID.");
                valid = false;
            }
        }
        if (!formData.label.trim()) {
            setError("Program label is required.");
            valid = false;
        }
        if (!formData.command.trim()) {
            setError("Program command is required.");
            valid = false;
        }
        return valid;
    };
    const handleSubmit = async (e)=>{
        e.preventDefault();
        if (!validateForm()) return;
        setError(null);
        setSuccess(null);
        const envMap = {};
        for (const ev of formData.envVars){
            if (ev.key.trim()) {
                envMap[ev.key.trim()] = ev.value;
            }
        }
        try {
            const client = getClient();
            await client.upsertProgramConfig({
                program: {
                    id: formData.id.trim(),
                    label: formData.label.trim(),
                    command: formData.command.trim(),
                    cliFlags: formData.cliFlags.trim(),
                    description: formData.description.trim(),
                    env: envMap,
                    isBuiltin: false
                }
            });
            setSuccess("Successfully ".concat(isEditing ? "updated" : "created", ' program "').concat(formData.label.trim(), '".'));
            setShowForm(false);
            setFormData(ProgramsManager_emptyForm);
            await fetchPrograms();
        } catch (err) {
            const msg = err instanceof Error ? err.message : String(err);
            setError("Failed to save program: ".concat(msg));
        }
    };
    const handleDelete = async (id)=>{
        setDeletingId(id);
        setError(null);
        setSuccess(null);
        try {
            const client = getClient();
            await client.deleteProgramConfig({
                id
            });
            setSuccess('Program "'.concat(id, '" deleted.'));
            setConfirmDeleteId(null);
            await fetchPrograms();
        } catch (err) {
            const msg = err instanceof Error ? err.message : String(err);
            setError("Failed to delete program: ".concat(msg));
        } finally{
            setDeletingId(null);
        }
    };
    const addEnvVar = ()=>{
        setFormData((prev)=>({
                ...prev,
                envVars: [
                    ...prev.envVars,
                    {
                        key: "",
                        value: ""
                    }
                ]
            }));
    };
    const updateEnvVar = (index, key, value)=>{
        setFormData((prev)=>{
            const updated = [
                ...prev.envVars
            ];
            updated[index] = {
                key,
                value
            };
            return {
                ...prev,
                envVars: updated
            };
        });
    };
    const removeEnvVar = (index)=>{
        setFormData((prev)=>({
                ...prev,
                envVars: prev.envVars.filter((_, i)=>i !== index)
            }));
    };
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: ProgramsManager_css_container,
        "data-testid": "programs-manager",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: ProgramsManager_css_headerRow,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                        className: ProgramsManager_css_heading,
                        children: "Program Configurations"
                    }),
                    !showForm && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: ProgramsManager_css_input,
                        style: {
                            maxWidth: "fit-content",
                            cursor: "pointer",
                            fontWeight: 600
                        },
                        onClick: handleOpenCreate,
                        "data-testid": "add-program-btn",
                        children: "+ Add Program"
                    })
                ]
            }),
            error && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: ProgramsManager_css_fieldError,
                "data-testid": "programs-error",
                children: error
            }),
            success && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                style: {
                    color: "#10b981",
                    fontSize: "0.875rem"
                },
                "data-testid": "programs-success",
                children: success
            }),
            showForm && /*#__PURE__*/ (0,jsx_runtime.jsxs)("form", {
                className: ProgramsManager_css_formCard,
                onSubmit: handleSubmit,
                "data-testid": "program-form",
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                        className: ProgramsManager_css_formTitle,
                        children: isEditing ? "Edit Program" : "Add Program"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: ProgramsManager_css_formFields,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: ProgramsManager_css_field,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                        className: ProgramsManager_css_label,
                                        htmlFor: "prog-id",
                                        children: "ID (unique identifier)"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                        id: "prog-id",
                                        type: "text",
                                        className: ProgramsManager_css_input,
                                        value: formData.id,
                                        disabled: isEditing,
                                        onChange: (e)=>setFormData({
                                                ...formData,
                                                id: e.target.value
                                            }),
                                        placeholder: "e.g. my-custom-agent",
                                        "data-testid": "prog-id-input"
                                    }),
                                    idError && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: ProgramsManager_css_fieldError,
                                        children: idError
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: ProgramsManager_css_field,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                        className: ProgramsManager_css_label,
                                        htmlFor: "prog-label",
                                        children: "Label (display name)"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                        id: "prog-label",
                                        type: "text",
                                        className: ProgramsManager_css_input,
                                        value: formData.label,
                                        onChange: (e)=>setFormData({
                                                ...formData,
                                                label: e.target.value
                                            }),
                                        placeholder: "e.g. My Custom Agent",
                                        "data-testid": "prog-label-input"
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: ProgramsManager_css_field,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                        className: ProgramsManager_css_label,
                                        htmlFor: "prog-command",
                                        children: "Executable Command / Path"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        className: commandRow,
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                id: "prog-command",
                                                type: "text",
                                                className: ProgramsManager_css_input,
                                                value: formData.command,
                                                onChange: (e)=>setFormData({
                                                        ...formData,
                                                        command: e.target.value
                                                    }),
                                                onBlur: ()=>probe.check(),
                                                onKeyDown: (e)=>{
                                                    if (e.key !== "Enter") return;
                                                    e.preventDefault();
                                                    probe.check({
                                                        immediate: true
                                                    });
                                                },
                                                autoCapitalize: "off",
                                                autoCorrect: "off",
                                                spellCheck: false,
                                                "aria-describedby": "prog-command-status-text",
                                                placeholder: "e.g. /usr/local/bin/my-agent or python -m myagent",
                                                "data-testid": "prog-command-input"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                ref: checkButtonRef,
                                                type: "button",
                                                className: checkButton,
                                                onClick: ()=>{
                                                    checkClicked.current = true;
                                                    probe.check({
                                                        explicit: true
                                                    });
                                                },
                                                disabled: probe.state.kind === "checking",
                                                "aria-busy": probe.state.kind === "checking",
                                                "data-testid": "prog-command-check",
                                                children: "Check"
                                            })
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)(ProbeStatusBadge/* ProbeStatusBadge */.q5, {
                                        state: probe.state,
                                        checkedToken: probe.checkedToken,
                                        onRetry: ()=>probe.check({
                                                immediate: true
                                            }),
                                        onConfirm: ()=>probe.check({
                                                explicit: true
                                            }),
                                        testId: "prog-command-status",
                                        id: "prog-command-status-text"
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: ProgramsManager_css_field,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                        className: ProgramsManager_css_label,
                                        htmlFor: "prog-flags",
                                        children: "Default CLI Flags (optional)"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                        onFocus: ()=>setFlagsFocused(true),
                                        onBlur: ()=>setFlagsFocused(false),
                                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)(FlagCombobox, {
                                            id: "prog-flags",
                                            className: ProgramsManager_css_input,
                                            value: formData.cliFlags,
                                            onChange: (cliFlags)=>setFormData({
                                                    ...formData,
                                                    cliFlags
                                                }),
                                            flags: probedFlags,
                                            placeholder: "e.g. --verbose --auto",
                                            testId: "prog-flags-input",
                                            describedBy: flagsDescribedBy
                                        })
                                    }),
                                    flagsHint && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        id: "prog-flags-hint",
                                        className: hintText,
                                        "data-testid": "prog-flags-hint",
                                        children: flagsHint
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)(UnknownFlagsWarning/* UnknownFlagsWarning */.u, {
                                        id: "prog-flags-warning",
                                        testId: "prog-flags-warning",
                                        program: probe.checkedToken,
                                        unknown: unknownFlags
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)(AvailableFlags, {
                                        flags: probedFlags,
                                        testId: "prog-available-flags"
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: ProgramsManager_css_field,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                        className: ProgramsManager_css_label,
                                        htmlFor: "prog-desc",
                                        children: "Description (optional)"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                        id: "prog-desc",
                                        type: "text",
                                        className: ProgramsManager_css_input,
                                        value: formData.description,
                                        onChange: (e)=>setFormData({
                                                ...formData,
                                                description: e.target.value
                                            }),
                                        placeholder: "e.g. Internal code review assistant",
                                        "data-testid": "prog-desc-input"
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: ProgramsManager_css_field,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                        className: ProgramsManager_css_label,
                                        children: "Environment Variables (optional)"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        className: ProgramsManager_css_envVarTable,
                                        children: [
                                            formData.envVars.map((ev, idx)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                    className: ProgramsManager_css_envVarRow,
                                                    children: [
                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                            type: "text",
                                                            className: ProgramsManager_css_envVarInput,
                                                            placeholder: "KEY",
                                                            value: ev.key,
                                                            onChange: (e)=>updateEnvVar(idx, e.target.value, ev.value)
                                                        }),
                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                                            type: "text",
                                                            className: ProgramsManager_css_envVarInput,
                                                            placeholder: "VALUE",
                                                            value: ev.value,
                                                            onChange: (e)=>updateEnvVar(idx, ev.key, e.target.value)
                                                        }),
                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                            type: "button",
                                                            className: ProgramsManager_css_deleteBtn,
                                                            onClick: ()=>removeEnvVar(idx),
                                                            children: "Remove"
                                                        })
                                                    ]
                                                }, idx)),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                type: "button",
                                                className: ProgramsManager_css_input,
                                                style: {
                                                    width: "fit-content",
                                                    cursor: "pointer",
                                                    fontSize: "0.75rem"
                                                },
                                                onClick: addEnvVar,
                                                children: "+ Add Env Var"
                                            })
                                        ]
                                    })
                                ]
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: ProgramsManager_css_formActions,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "submit",
                                className: ProgramsManager_css_input,
                                style: {
                                    backgroundColor: "#2563eb",
                                    color: "#ffffff",
                                    fontWeight: 600,
                                    cursor: "pointer"
                                },
                                "data-testid": "save-program-btn",
                                children: "Save Program"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: ProgramsManager_css_input,
                                style: {
                                    cursor: "pointer"
                                },
                                onClick: handleCancelForm,
                                "data-testid": "cancel-program-btn",
                                children: "Cancel"
                            })
                        ]
                    })
                ]
            }),
            loading ? /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: ProgramsManager_css_loadingText,
                children: "Loading programs..."
            }) : programs.length === 0 ? /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: ProgramsManager_css_emptyText,
                children: "No programs configured."
            }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                style: {
                    display: "flex",
                    flexDirection: "column",
                    gap: "0.5rem"
                },
                children: programs.map((p)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: programRow,
                        "data-testid": "program-row-".concat(p.id),
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: programInfo,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        className: programTitleRow,
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: programLabel,
                                                children: p.label
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: p.isBuiltin ? badgeBuiltin : badgeCustom,
                                                children: p.isBuiltin ? "Built-in" : "Custom"
                                            })
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        className: programDesc,
                                        children: [
                                            "Command: ",
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("code", {
                                                children: p.command
                                            }),
                                            " ",
                                            p.cliFlags && " (Flags: ".concat(p.cliFlags, ")")
                                        ]
                                    }),
                                    p.description && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                        className: programMeta,
                                        children: p.description
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: programActions,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        type: "button",
                                        className: ProgramsManager_css_input,
                                        style: {
                                            padding: "0.25rem 0.5rem",
                                            fontSize: "0.75rem",
                                            cursor: "pointer"
                                        },
                                        onClick: ()=>handleDuplicate(p),
                                        "data-testid": "duplicate-program-".concat(p.id),
                                        children: "Duplicate"
                                    }),
                                    !p.isBuiltin && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                type: "button",
                                                className: ProgramsManager_css_input,
                                                style: {
                                                    padding: "0.25rem 0.5rem",
                                                    fontSize: "0.75rem",
                                                    cursor: "pointer"
                                                },
                                                onClick: ()=>handleOpenEdit(p),
                                                "data-testid": "edit-program-".concat(p.id),
                                                children: "Edit"
                                            }),
                                            confirmDeleteId === p.id ? /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                type: "button",
                                                className: ProgramsManager_css_confirmDeleteBtn,
                                                disabled: deletingId === p.id,
                                                onClick: ()=>handleDelete(p.id),
                                                "data-testid": "confirm-delete-program-".concat(p.id),
                                                children: "Confirm Delete"
                                            }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                type: "button",
                                                className: ProgramsManager_css_deleteBtn,
                                                onClick: ()=>setConfirmDeleteId(p.id),
                                                "data-testid": "delete-program-".concat(p.id),
                                                children: "Delete"
                                            })
                                        ]
                                    })
                                ]
                            })
                        ]
                    }, p.id))
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/lib/hooks/usePushNotifications.ts
var usePushNotifications = __webpack_require__(13468);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsettings%2FPushNotificationSettings.css.ts.vanilla.css%22%2C%22source%22%3A%22Ll8xNGw1MW15MCB7CiAgZGlzcGxheTogZmxleDsKICBmbGV4LWRpcmVjdGlvbjogY29sdW1uOwogIGdhcDogMC43NXJlbTsKfQouXzE0bDUxbXkxIHsKICBmb250LXNpemU6IHZhcigtLV8xOWV6MnJ6M2QpOwogIGZvbnQtd2VpZ2h0OiA2MDA7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MCk7CiAgbWFyZ2luOiAwIDAgMC4yNXJlbTsKfQouXzE0bDUxbXkyIHsKICBmb250LXNpemU6IHZhcigtLV8xOWV6MnJ6M2MpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyejEpOwogIG1hcmdpbjogMDsKfQouXzE0bDUxbXkzIHsKICBkaXNwbGF5OiBpbmxpbmUtYmxvY2s7CiAgcGFkZGluZzogdmFyKC0tXzE5ZXoycnoyeSkgMC42MjVyZW07CiAgYm9yZGVyLXJhZGl1czogdmFyKC0tXzE5ZXoycnozOSk7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNhKTsKICBmb250LXdlaWdodDogNjAwOwp9Ci5fMTRsNTFteTQgewogIGJhY2tncm91bmQ6IHZhcigtLV8xOWV6MnJ6MTEpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyejEyKTsKfQouXzE0bDUxbXk1IHsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyejgpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyejIpOwp9Ci5fMTRsNTFteTYgewogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYyk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MSk7CiAgbWFyZ2luOiAwLjI1cmVtIDAgMDsKfQouXzE0bDUxbXk3IHsKICBkaXNwbGF5OiBmbGV4OwogIGFsaWduLWl0ZW1zOiBjZW50ZXI7CiAgZ2FwOiAwLjc1cmVtOwp9Ci5fMTRsNTFteTggewogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYyk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MCk7CiAgY3Vyc29yOiBwb2ludGVyOwogIHVzZXItc2VsZWN0OiBub25lOwp9Ci5fMTRsNTFteTkgewogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYyk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MTApOwogIG1hcmdpbjogMDsKfQ%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsettings_2FPushNotificationSettings_css_ts_vanilla_css_22_2C_22source_22_3A_22Ll8xNGw1MW15MCB7CiAgZGlzcGxheTogZmxleDsKICBmbGV4LWRpcmVjdGlvbjogY29sdW1uOwogIGdhcDogMC43NXJlbTsKfQouXzE0bDUxbXkxIHsKICBmb250LXNpemU6IHZhcigtLV8xOWV6MnJ6M2QpOwogIGZvbnQtd2VpZ2h0OiA2MDA7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MCk7CiAgbWFyZ2luOiAwIDAgMC4yNXJlbTsKfQouXzE0bDUxbXkyIHsKICBmb250LXNpemU6IHZhcigtLV8xOWV6MnJ6M2MpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyejEpOwogIG1hcmdpbjogMDsKfQouXzE0bDUxbXkzIHsKICBkaXNwbGF5OiBpbmxpbmUtYmxvY2s7CiAgcGFkZGluZzogdmFyKC0tXzE5ZXoycnoyeSkgMC42MjVyZW07CiAgYm9yZGVyLXJhZGl1czogdmFyKC0tXzE5ZXoycnozOSk7CiAgZm9udC1zaXplOiB2YXIoLS1fMTllejJyejNhKTsKICBmb250LXdlaWdodDogNjAwOwp9Ci5fMTRsNTFteTQgewogIGJhY2tncm91bmQ6IHZhcigtLV8xOWV6MnJ6MTEpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyejEyKTsKfQouXzE0bDUxbXk1IHsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyejgpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyejIpOwp9Ci5fMTRsNTFteTYgewogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYyk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MSk7CiAgbWFyZ2luOiAwLjI1cmVtIDAgMDsKfQouXzE0bDUxbXk3IHsKICBkaXNwbGF5OiBmbGV4OwogIGFsaWduLWl0ZW1zOiBjZW50ZXI7CiAgZ2FwOiAwLjc1cmVtOwp9Ci5fMTRsNTFteTggewogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYyk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MCk7CiAgY3Vyc29yOiBwb2ludGVyOwogIHVzZXItc2VsZWN0OiBub25lOwp9Ci5fMTRsNTFteTkgewogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYyk7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MTApOwogIG1hcmdpbjogMDsKfQ_22_7D = __webpack_require__(4869);
;// ./src/components/settings/PushNotificationSettings.css.ts


var PushNotificationSettings_css_container = '_14l51my0';
var description = '_14l51my2';
var errorText = '_14l51my9';
var instructions = '_14l51my6';
var statusBadge = '_14l51my3';
var statusBlocked = '_14l51my4';
var statusUnsupported = '_14l51my5';
var title = '_14l51my1';
var toggleLabel = '_14l51my8';
var toggleRow = '_14l51my7';
;// ./src/components/settings/PushNotificationSettings.tsx
/* __next_internal_client_entry_do_not_use__ PushNotificationSettings auto */ 



function PushNotificationSettings() {
    const { isSupported, subscription, isLoading, error, permission, subscribe, unsubscribe } = (0,usePushNotifications/* usePushNotifications */.c$)();
    const isSubscribed = subscription !== null;
    const baseUrl = (0,config/* getApiBaseUrl */.h)();
    const handleToggle = async ()=>{
        if (isSubscribed) {
            await unsubscribe(baseUrl);
        } else {
            await subscribe(baseUrl);
        }
    };
    if (isLoading) {
        return null;
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: PushNotificationSettings_css_container,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                className: title,
                children: "Push Notifications"
            }),
            !isSupported && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: "".concat(statusBadge, " ").concat(statusUnsupported),
                        children: "Not supported"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: description,
                        children: "Push notifications are not supported in this browser."
                    })
                ]
            }),
            isSupported && permission === "denied" && /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: "".concat(statusBadge, " ").concat(statusBlocked),
                        children: "Blocked"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("p", {
                        className: instructions,
                        children: [
                            "Notifications are blocked in your browser settings. To enable them, open your browser's site settings for this page and set Notifications to",
                            " ",
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("strong", {
                                children: "Allow"
                            }),
                            "."
                        ]
                    })
                ]
            }),
            isSupported && permission !== "denied" && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: toggleRow,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                        id: "push-notifications-toggle",
                        type: "checkbox",
                        checked: isSubscribed,
                        onChange: handleToggle
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                        htmlFor: "push-notifications-toggle",
                        className: toggleLabel,
                        children: isSubscribed ? "Push notifications enabled" : "Enable push notifications"
                    })
                ]
            }),
            error && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: errorText,
                children: error
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/wkt/timestamp.js
var timestamp = __webpack_require__(28765);
// EXTERNAL MODULE: ./src/lib/utils/datetime.ts
var datetime = __webpack_require__(3335);
// EXTERNAL MODULE: ./src/components/common/InlineNotice.tsx + 1 modules
var InlineNotice = __webpack_require__(96307);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2FsettingsPanelForm.css.ts.vanilla.css%22%2C%22source%22%3A%22LmFvZ2NhbTAgewogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBnYXA6IDFyZW07CiAgbWF4LXdpZHRoOiA2NDBweDsKfQouYW9nY2FtMSB7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MCk7CiAgZm9udC1zaXplOiAxLjI1cmVtOwogIGZvbnQtd2VpZ2h0OiA2MDA7CiAgbWFyZ2luOiAwOwp9Ci5hb2djYW0yIHsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoyKTsKICBmb250LXNpemU6IDAuODc1cmVtOwogIG1hcmdpbjogMDsKfQouYW9nY2FtMyB7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6Mik7Cn0KLmFvZ2NhbTQgewogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBnYXA6IDEuMjVyZW07Cn0KLmFvZ2NhbTUgewogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBnYXA6IDAuNXJlbTsKfQouYW9nY2FtNiB7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MSk7CiAgZm9udC1zaXplOiAwLjg3NXJlbTsKICBmb250LXdlaWdodDogNjAwOwp9Ci5hb2djYW03IHsKICBkaXNwbGF5OiBmbGV4OwogIGdhcDogMC41cmVtOwogIGFsaWduLWl0ZW1zOiBjZW50ZXI7Cn0KLmFvZ2NhbTggewogIHBhZGRpbmc6IDAuNXJlbSAwLjc1cmVtOwogIGJhY2tncm91bmQtY29sb3I6IHZhcigtLV8xOWV6MnJ6MWEpOwogIGJvcmRlcjogMXB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6ayk7CiAgYm9yZGVyLXJhZGl1czogNHB4OwogIGNvbG9yOiB2YXIoLS1fMTllejJyejFiKTsKICBmb250LXNpemU6IDAuODc1cmVtOwogIGZsZXg6IDE7Cn0KLmFvZ2NhbTg6Zm9jdXMgewogIG91dGxpbmU6IG5vbmU7CiAgYm9yZGVyLWNvbG9yOiB2YXIoLS1fMTllejJyemwpOwp9%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2FsettingsPanelForm_css_ts_vanilla_css_22_2C_22source_22_3A_22LmFvZ2NhbTAgewogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBnYXA6IDFyZW07CiAgbWF4LXdpZHRoOiA2NDBweDsKfQouYW9nY2FtMSB7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MCk7CiAgZm9udC1zaXplOiAxLjI1cmVtOwogIGZvbnQtd2VpZ2h0OiA2MDA7CiAgbWFyZ2luOiAwOwp9Ci5hb2djYW0yIHsKICBjb2xvcjogdmFyKC0tXzE5ZXoycnoyKTsKICBmb250LXNpemU6IDAuODc1cmVtOwogIG1hcmdpbjogMDsKfQouYW9nY2FtMyB7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6Mik7Cn0KLmFvZ2NhbTQgewogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBnYXA6IDEuMjVyZW07Cn0KLmFvZ2NhbTUgewogIGRpc3BsYXk6IGZsZXg7CiAgZmxleC1kaXJlY3Rpb246IGNvbHVtbjsKICBnYXA6IDAuNXJlbTsKfQouYW9nY2FtNiB7CiAgY29sb3I6IHZhcigtLV8xOWV6MnJ6MSk7CiAgZm9udC1zaXplOiAwLjg3NXJlbTsKICBmb250LXdlaWdodDogNjAwOwp9Ci5hb2djYW03IHsKICBkaXNwbGF5OiBmbGV4OwogIGdhcDogMC41cmVtOwogIGFsaWduLWl0ZW1zOiBjZW50ZXI7Cn0KLmFvZ2NhbTggewogIHBhZGRpbmc6IDAuNXJlbSAwLjc1cmVtOwogIGJhY2tncm91bmQtY29sb3I6IHZhcigtLV8xOWV6MnJ6MWEpOwogIGJvcmRlcjogMXB4IHNvbGlkIHZhcigtLV8xOWV6MnJ6ayk7CiAgYm9yZGVyLXJhZGl1czogNHB4OwogIGNvbG9yOiB2YXIoLS1fMTllejJyejFiKTsKICBmb250LXNpemU6IDAuODc1cmVtOwogIGZsZXg6IDE7Cn0KLmFvZ2NhbTg6Zm9jdXMgewogIG91dGxpbmU6IG5vbmU7CiAgYm9yZGVyLWNvbG9yOiB2YXIoLS1fMTllejJyemwpOwp9_22_7D = __webpack_require__(33545);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsettings%2FSlackNotificationSettings.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA71U23KDIBR89yt4rA9kvMTakI_poBDDhAADGBM7-feO2DpqtEmctq-HPXv2LAurOj-zdRqADw-ATGpCNcwllxqBE9YvEL6HG1pHug4Df-tdvVWLDx1-Ehj5Ww-AnRQWGlZTBILVWxglmh6b-hHrggkEgh5ZNE8WPs8WOzaFCWGiaPBx2sBBsEq_2zKcHwotS0Fmdg3d1NYOBEJ1BkZyRqYs6VzTmLDSILBW5-3CdfJSm6ZJSSYs1U2p2jNLoVE4pwgIWWms-ruivTxR7TaWCufMXhrKzQBCmMEZp2SMSvojhbQQcy4rSnrN61_wMh54Gc15GT_rZfKjla5eUVbs7bgxZv4isxNnBmFGcXxBYMepE4c5KwRklh5NW4TGYm2bowIrZ7TT1PG8zsc9mNgpHbWnQxkZl_lh1qSJvI353u6-vv94Lr2MjRM2fcWd_s28_uoh-aeH1Jd_Ih4v_Ue_hFip5iVTvyeuhd5ef3bn-jtgPh3_QcZvpo2GkYU__fUTxJ6exqcGAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsettings_2FSlackNotificationSettings_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA71U23KDIBR89yt4rA9kvMTakI_poBDDhAADGBM7_feO2DpqtEmctq_HPXv2LAurOj_zdRqADw_ATGpCNcwllxqBE9YvEL6HG1pHug4Df_tdvVWLDx1_Ehj5Ww_AnRQWGlZTBILVWxglmh6b_hHrggkEgh5ZNE8WPs8WOzaFCWGiaPBx2sBBsEq_2zKcHwotS0Fmdg3d1NYOBEJ1BkZyRqYs6VzTmLDSILBW5_3CdfJSm6ZJSSYs1U2p2jNLoVE4pwgIWWms_ruivTxR7TaWCufMXhrKzQBCmMEZp2SMSvojhbQQcy4rSnrN61_wMh54Gc15GT_rZfKjla5eUVbs7bgxZv4isxNnBmFGcXxBYMepE4c5KwRklh5NW4TGYm2bowIrZ7TT1PG8zsc9mNgpHbWnQxkZl_lh1qSJvI353u6_vv94Lr2MjRM2fcWd_s28_uoh_aeH1Jd_Ih4v_Ue_hFip5iVTvyeuhd5ef3bn_jtgPh3_QcZvpo2GkYU_fUTxJ6exqcGAAA_22_7D = __webpack_require__(5185);
;// ./src/components/settings/SlackNotificationSettings.css.ts



var SlackNotificationSettings_css_actions = 'zcxi47c';
var betaNote = 'zcxi477';
var confirmRemoveBtn = 'zcxi474';
var SlackNotificationSettings_css_container = 'aogcam0';
var deliveryStatus = 'zcxi47a';
var deliveryStatusFailed = 'zcxi47b';
var SlackNotificationSettings_css_description = 'aogcam2';
var SlackNotificationSettings_css_errorText = 'zcxi472';
var SlackNotificationSettings_css_field = 'aogcam5';
var SlackNotificationSettings_css_form = 'aogcam4';
var SlackNotificationSettings_css_heading = 'aogcam1';
var SlackNotificationSettings_css_hint = 'zcxi471';
var SlackNotificationSettings_css_input = 'aogcam8';
var inputInvalid = 'zcxi470';
var inputRow = 'aogcam7';
var SlackNotificationSettings_css_label = 'aogcam6';
var SlackNotificationSettings_css_loadingText = 'aogcam3';
var removeBtn = 'zcxi473';
var SlackNotificationSettings_css_saveError = 'zcxi47d';
var testResultAlert = 'zcxi478';
var testResultStatus = 'zcxi479';
var SlackNotificationSettings_css_toggleLabel = 'zcxi476';
var SlackNotificationSettings_css_toggleRow = 'zcxi475';
;// ./src/components/settings/SlackNotificationSettings.tsx
/* __next_internal_client_entry_do_not_use__ SlackNotificationSettings auto */ 
// +feature: slack-notification-settings









// WEBHOOK_URL_PATTERN mirrors the server-side backstop in
// server/services/slack_config_service.go's slackWebhookURLPrefix check.
const WEBHOOK_URL_PATTERN = /^https:\/\/hooks\.slack\.com\/services\//;
const WEBHOOK_URL_ERROR = "This doesn't look like a Slack Incoming Webhook URL (expected https://hooks.slack.com/services/...)";
function SlackNotificationSettings() {
    const [loading, setLoading] = (0,react.useState)(true);
    const [loadError, setLoadError] = (0,react.useState)(null);
    const [webhookConfigured, setWebhookConfigured] = (0,react.useState)(false);
    const [webhookInput, setWebhookInput] = (0,react.useState)("");
    const [webhookError, setWebhookError] = (0,react.useState)(null);
    const [removeConfirming, setRemoveConfirming] = (0,react.useState)(false);
    const [notifyOnQueueItem, setNotifyOnQueueItem] = (0,react.useState)(false);
    const [approvalEnabled, setApprovalEnabled] = (0,react.useState)(false);
    const [queueDepthThreshold, setQueueDepthThreshold] = (0,react.useState)(0);
    const [dashboardBaseUrl, setDashboardBaseUrl] = (0,react.useState)("");
    const [dashboardWarningDismissed, setDashboardWarningDismissed] = (0,react.useState)(false);
    const [lastDelivery, setLastDelivery] = (0,react.useState)(null);
    const [testSubmitting, setTestSubmitting] = (0,react.useState)(false);
    const [testResult, setTestResult] = (0,react.useState)(null);
    const [saving, setSaving] = (0,react.useState)(false);
    const [saveError, setSaveError] = (0,react.useState)(null);
    const clientRef = (0,react.useRef)(null);
    const loadConfig = (0,react.useCallback)(async ()=>{
        if (!clientRef.current) return;
        try {
            setLoading(true);
            setLoadError(null);
            const response = await clientRef.current.getSlackConfig({});
            const cfg = response.config;
            if (cfg) {
                setWebhookConfigured(cfg.webhookConfigured);
                setNotifyOnQueueItem(cfg.notifyOnQueueItem);
                setApprovalEnabled(cfg.approvalEnabled);
                setQueueDepthThreshold(cfg.queueDepthThreshold);
                setDashboardBaseUrl(cfg.dashboardBaseUrl);
                if (cfg.lastDelivery) {
                    setLastDelivery({
                        attempted: cfg.lastDelivery.attempted,
                        success: cfg.lastDelivery.success,
                        error: cfg.lastDelivery.error,
                        attemptedAt: cfg.lastDelivery.attemptedAt ? (0,timestamp/* timestampDate */.FP)(cfg.lastDelivery.attemptedAt) : undefined
                    });
                } else {
                    setLastDelivery(null);
                }
            }
        } catch (e) {
            setLoadError("Couldn't load Slack settings.");
        } finally{
            setLoading(false);
        }
    }, []);
    (0,react.useEffect)(()=>{
        const transport = (0,connect_transport/* createConnectTransport */.w)({
            baseUrl: (0,config/* getApiBaseUrl */.h)()
        });
        clientRef.current = (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, transport);
        loadConfig();
    }, [
        loadConfig
    ]);
    const trimmedWebhook = webhookInput.trim();
    const hasValidTypedWebhook = WEBHOOK_URL_PATTERN.test(trimmedWebhook);
    const togglesEnabled = webhookConfigured || hasValidTypedWebhook;
    const testDisabled = !togglesEnabled || testSubmitting;
    function handleWebhookChange(e) {
        const value = e.target.value;
        setWebhookInput(value);
        const trimmed = value.trim();
        // Clear the error live as soon as the shape becomes valid (or the field
        // is emptied) — no re-submit required to see it clear (design/ux.md's
        // error-handling table).
        if (webhookError && (trimmed === "" || WEBHOOK_URL_PATTERN.test(trimmed))) {
            setWebhookError(null);
        }
    }
    function handleWebhookBlur() {
        const trimmed = webhookInput.trim();
        if (trimmed === "") {
            setWebhookError(null);
            return;
        }
        setWebhookError(WEBHOOK_URL_PATTERN.test(trimmed) ? null : WEBHOOK_URL_ERROR);
    }
    async function handleTestSend() {
        if (!clientRef.current) return;
        setTestSubmitting(true);
        setTestResult(null);
        try {
            const resp = await clientRef.current.testSlackWebhook({
                webhookUrl: trimmedWebhook
            });
            if (resp.success) {
                setTestResult({
                    success: true,
                    message: "Test message sent — check your Slack channel."
                });
            } else {
                setTestResult({
                    success: false,
                    message: "Test failed: ".concat(resp.error)
                });
            }
        } catch (err) {
            setTestResult({
                success: false,
                message: "Test failed: ".concat(err instanceof Error ? err.message : String(err))
            });
        } finally{
            setTestSubmitting(false);
        }
    }
    async function handleRemoveWebhook() {
        if (!clientRef.current) return;
        setSaving(true);
        setSaveError(null);
        try {
            await clientRef.current.updateSlackConfig({
                clearWebhookUrl: true,
                notifyOnQueueItem: false,
                queueDepthThreshold,
                approvalEnabled: false,
                dashboardBaseUrl: dashboardBaseUrl.trim()
            });
            setWebhookInput("");
            setTestResult(null);
            setRemoveConfirming(false);
            await loadConfig();
        } catch (err) {
            setSaveError(err instanceof Error ? err.message : String(err));
        } finally{
            setSaving(false);
        }
    }
    async function handleSave() {
        if (!clientRef.current || webhookError) return;
        setSaving(true);
        setSaveError(null);
        try {
            await clientRef.current.updateSlackConfig({
                webhookUrl: trimmedWebhook,
                notifyOnQueueItem,
                queueDepthThreshold,
                approvalEnabled,
                dashboardBaseUrl: dashboardBaseUrl.trim()
            });
            setWebhookInput("");
            await loadConfig();
        } catch (err) {
            setSaveError(err instanceof Error ? err.message : String(err));
        } finally{
            setSaving(false);
        }
    }
    const showDashboardWarning = notifyOnQueueItem && dashboardBaseUrl.trim() === "" && !dashboardWarningDismissed;
    if (loading) {
        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            className: SlackNotificationSettings_css_container,
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                    className: SlackNotificationSettings_css_heading,
                    children: "Slack Notifications"
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                    className: SlackNotificationSettings_css_loadingText,
                    children: "Loading…"
                })
            ]
        });
    }
    if (loadError) {
        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
            className: SlackNotificationSettings_css_container,
            children: [
                /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                    className: SlackNotificationSettings_css_heading,
                    children: "Slack Notifications"
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    role: "alert",
                    className: testResultAlert,
                    children: loadError
                }),
                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                    className: SlackNotificationSettings_css_actions,
                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        className: "btn btn-primary",
                        onClick: loadConfig,
                        children: "Retry"
                    })
                })
            ]
        });
    }
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: SlackNotificationSettings_css_container,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                className: SlackNotificationSettings_css_heading,
                children: "Slack Notifications"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                className: SlackNotificationSettings_css_description,
                children: "Get pinged in Slack when an agent needs your review or approval."
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: SlackNotificationSettings_css_form,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: SlackNotificationSettings_css_field,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                htmlFor: "slack-webhook-url",
                                className: SlackNotificationSettings_css_label,
                                children: "Webhook URL"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: inputRow,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                        id: "slack-webhook-url",
                                        type: "text",
                                        className: "".concat(SlackNotificationSettings_css_input, " ").concat(webhookError ? inputInvalid : ""),
                                        value: webhookInput,
                                        onChange: handleWebhookChange,
                                        onBlur: handleWebhookBlur,
                                        placeholder: webhookConfigured ? "•••• (configured)" : "https://hooks.slack.com/services/...",
                                        "aria-describedby": webhookError ? "slack-webhook-hint slack-webhook-error" : "slack-webhook-hint",
                                        "aria-invalid": webhookError ? "true" : "false"
                                    }),
                                    webhookConfigured && (removeConfirming ? /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        type: "button",
                                        className: confirmRemoveBtn,
                                        onClick: handleRemoveWebhook,
                                        disabled: saving,
                                        "data-testid": "slack-webhook-confirm-remove",
                                        children: "Confirm remove?"
                                    }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        type: "button",
                                        className: removeBtn,
                                        onClick: ()=>setRemoveConfirming(true),
                                        disabled: saving,
                                        "data-testid": "slack-webhook-remove",
                                        children: "Remove"
                                    }))
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                id: "slack-webhook-hint",
                                className: SlackNotificationSettings_css_hint,
                                children: "Paste your Slack Incoming Webhook URL"
                            }),
                            webhookError && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                id: "slack-webhook-error",
                                role: "alert",
                                "data-testid": "slack-webhook-error",
                                className: SlackNotificationSettings_css_errorText,
                                children: webhookError
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: "btn btn-secondary",
                                onClick: handleTestSend,
                                disabled: testDisabled,
                                children: testSubmitting ? "Sending…" : "Send test message"
                            }),
                            testResult && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                role: testResult.success ? "status" : "alert",
                                "data-testid": "slack-test-webhook-result",
                                className: testResult.success ? testResultStatus : testResultAlert,
                                children: testResult.message
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: SlackNotificationSettings_css_toggleRow,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                id: "slack-notify-queue-item",
                                type: "checkbox",
                                checked: notifyOnQueueItem,
                                onChange: (e)=>setNotifyOnQueueItem(e.target.checked),
                                disabled: !togglesEnabled
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                htmlFor: "slack-notify-queue-item",
                                className: SlackNotificationSettings_css_toggleLabel,
                                children: "Notify on new review-queue item"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: SlackNotificationSettings_css_field,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                htmlFor: "slack-queue-depth-threshold",
                                className: SlackNotificationSettings_css_label,
                                children: "Queue-depth digest threshold (0 = off)"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                id: "slack-queue-depth-threshold",
                                type: "number",
                                min: 0,
                                className: SlackNotificationSettings_css_input,
                                value: queueDepthThreshold,
                                onChange: (e)=>setQueueDepthThreshold(Math.max(0, parseInt(e.target.value, 10) || 0)),
                                disabled: !togglesEnabled,
                                "aria-describedby": "slack-queue-depth-threshold-hint"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                id: "slack-queue-depth-threshold-hint",
                                className: SlackNotificationSettings_css_hint,
                                children: "You'll get one digest per burst; a persistently full queue won't nudge you again until it drops below the threshold and re-crosses."
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: SlackNotificationSettings_css_toggleRow,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                id: "slack-approval-enabled",
                                type: "checkbox",
                                checked: approvalEnabled,
                                onChange: (e)=>setApprovalEnabled(e.target.checked),
                                disabled: !togglesEnabled,
                                "aria-describedby": "slack-approval-enabled-hint"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                htmlFor: "slack-approval-enabled",
                                className: SlackNotificationSettings_css_toggleLabel,
                                children: "Allow Approve/Deny from Slack"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        id: "slack-approval-enabled-hint",
                        className: betaNote,
                        children: "Beta — requires public reachability (stapler-squad must be reachable from Slack's servers); see docs/how-to/expose-slack-interactive-endpoint.md"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: SlackNotificationSettings_css_field,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("label", {
                                htmlFor: "slack-dashboard-base-url",
                                className: SlackNotificationSettings_css_label,
                                children: "Dashboard URL"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                                id: "slack-dashboard-base-url",
                                type: "text",
                                className: SlackNotificationSettings_css_input,
                                value: dashboardBaseUrl,
                                onChange: (e)=>setDashboardBaseUrl(e.target.value),
                                "aria-describedby": "slack-dashboard-base-url-hint"
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                id: "slack-dashboard-base-url-hint",
                                className: SlackNotificationSettings_css_hint,
                                children: 'Used to build "view in dashboard" links in Slack messages. Leave blank and links will only work on your home network.'
                            })
                        ]
                    }),
                    showDashboardWarning && /*#__PURE__*/ (0,jsx_runtime.jsx)(InlineNotice/* InlineNotice */.w, {
                        message: "Your Slack links may not work outside your home network — set a Dashboard URL below.",
                        onDismiss: ()=>setDashboardWarningDismissed(true),
                        "data-testid": "slack-dashboard-warning"
                    }),
                    saveError && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        role: "alert",
                        className: SlackNotificationSettings_css_saveError,
                        children: saveError
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: SlackNotificationSettings_css_actions,
                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                            type: "button",
                            className: "btn btn-primary",
                            onClick: handleSave,
                            disabled: saving || Boolean(webhookError),
                            children: saving ? "Saving…" : "Save"
                        })
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        role: "status",
                        "data-testid": "slack-last-delivery-status",
                        className: "".concat(deliveryStatus, " ").concat(lastDelivery && lastDelivery.attempted && !lastDelivery.success ? deliveryStatusFailed : ""),
                        children: lastDelivery && lastDelivery.attempted ? /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                            children: [
                                "Last Slack delivery:",
                                " ",
                                lastDelivery.attemptedAt ? (0,datetime/* formatRelativeTime */.fw)(lastDelivery.attemptedAt.getTime()) : "unknown",
                                " ",
                                "—",
                                " ",
                                lastDelivery.success ? "✓ delivered" : "✗ failed: ".concat(lastDelivery.error)
                            ]
                        }) : "Last Slack delivery: none yet"
                    })
                ]
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/lib/contexts/ThemeContext.tsx + 1 modules
var ThemeContext = __webpack_require__(45308);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsettings%2FThemePicker.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA7WWUW_aMBDH3_kUlqZKIOHKdqBA-rJvUh3xOXhL7Mx2aMbU7z7FSSqSFti0YT9A4vPd7_7n6Pz4wo-JV5uMkV8zQqT2VQE_U6IKbJ5nJP5SqR1mQVuTkswWdWnalRyqlBzBzSl94Ts8CXdK-OJ59jZ7HHzy6LMEl2uTEhb9WROo1yf8sFcu3tdfUeeH8MHiEC0yW1g3XWPjwOJvAu8vu53kk4w1yp2WUQmnJQ1YVgUEpJ1CPiUOK4QwhzpYqnRRLEmpTQnNnD-xqlkSrtxicUnJSUKrGLmyXndlcFhA0Ed8_ueaiVNkqEBKbfJPOQjZWyfRpURUDfG20HJihmdW1IHUtf_gadvZQPY9d7Y2cmqw6cpQO9_WobLaBHTtq4BNoFDo3KSkQBXiOwdm0KIPGwtI2CNfe4LgcUn2tqH-ANK-nr0ey5oe7BFdFPfczZRN9_kN_lLC4uRVM5VzPSlcqmxWe3rUXu8LjJFsHQpt8LKcZQzXm1GrlMcQrUeu1zexyz_HXi2WRBuP4aJJOU7sKUY_9F_qals1t4_A5sMRaBMER_N2A5ow58laYr4kX1gchD20_4ELxsi6e2BJwhjhjD2MjiZ_17I3GdFu7kwrmeTQ0XLBhYCelgOHBK_SKiXkZjui3d6ZNmPAtmQAXPNB20QmTFzXtt0qxtru7kyrmOL8nRa46mlFO-Aq7SZLAOWIFu5Lq-LoaNWunT2tavO4ri2ydo5o93fWFtp5pu1wbgW08yptEseINou0tzrt_2nx8kYsuOxJjD3hpLfC3tuiDrG3BnupYbrP-Pu1Vy3DISWCdeUayjc8Xy_f7kafLC8n9mPx6X0g9k6qA5Y-JRkOffVb7YNWP2lmTUATzpfOZOU99LWq6Sjo1xKlBjIvoaG9Ak8rVjWLqO_kEnXr1iS6-1Eb-W329hvjd1CRpgoAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsettings_2FThemePicker_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA7WWUW_aMBDH3_kUlqZKIOHKdqBA_rJvUh3xOXhL7Mx2aMbU7z7FSSqSFti0YT9A4vPd7_7n6Pz4wo_JV5uMkV8zQqT2VQE_U6IKbJ5nJP5SqR1mQVuTkswWdWnalRyqlBzBzSl94Ts8CXdK_OJ59jZ7HHzy6LMEl2uTEhb9WROo1yf8sFcu3tdfUeeH8MHiEC0yW1g3XWPjwOJvAu8vu53kk4w1yp2WUQmnJQ1YVgUEpJ1CPiUOK4QwhzpYqnRRLEmpTQnNnD_xqlkSrtxicUnJSUKrGLmyXndlcFhA0Ed8_ueaiVNkqEBKbfJPOQjZWyfRpURUDfG20HJihmdW1IHUtf_gadvZQPY9d7Y2cmqw6cpQO9_WobLaBHTtq4BNoFDo3KSkQBXiOwdm0KIPGwtI2CNfe4LgcUn2tqH_ANK_nr0ey5oe7BFdFPfczZRN9_kN_lLC4uRVM5VzPSlcqmxWe3rUXu8LjJFsHQpt8LKcZQzXm1GrlMcQrUeu1zexyz_HXi2WRBuP4aJJOU7sKUY_9F_qals1t4_A5sMRaBMER_N2A5ow58laYr4kX1gchD20_4ELxsi6e2BJwhjhjD2MjiZ_17I3GdFu7kwrmeTQ0XLBhYCelgOHBK_SKiXkZjui3d6ZNmPAtmQAXPNB20QmTFzXtt0qxtru7kyrmOL8nRa46mlFO_Aq7SZLAOWIFu5Lq_LoaNWunT2tavO4ri2ydo5o93fWFtp5pu1wbgW08yptEseINou0tzrt_2nx8kYsuOxJjD3hpLfC3tuiDrG3BnupYbrP_Pu1Vy3DISWCdeUayjc8Xy_f7kafLC8n9mPx6X0g9k6qA5Y_JRkOffVb7YNWP2lmTUATzpfOZOU99LWq6Sjo1xKlBjIvoaG9Ak8rVjWLqO_kEnXr1iS6_1Eb_W329hvjd1CRpgoAAA_22_7D = __webpack_require__(52748);
;// ./src/components/settings/ThemePicker.css.ts


var activeCheckmark = '_1v3sf7ce';
var ThemePicker_css_container = '_1v3sf7c0';
var grid = '_1v3sf7c3';
var previewSwatch = {matrix:'_1v3sf7c6',cyberpunk77:'_1v3sf7c7',wh40k:'_1v3sf7c8',clean:'_1v3sf7c9',light:'_1v3sf7ca',dark:'_1v3sf7cb'};
var sectionDescription = '_1v3sf7c2';
var sectionTitle = '_1v3sf7c1';
var themeButton = '_1v3sf7c4';
var themeButtonActive = '_1v3sf7c5';
var themeDescription = '_1v3sf7cd';
var themeName = '_1v3sf7cc';
;// ./src/components/settings/ThemePicker.tsx
/* __next_internal_client_entry_do_not_use__ ThemePicker auto */ 
// +feature: settings-theme-picker


/** Human-readable metadata for each theme. */ const THEME_META = {
    matrix: {
        label: "Matrix",
        description: "Green on black, JetBrains Mono"
    },
    cyberpunk77: {
        label: "Cyberpunk 77",
        description: "Neon yellow + pink on navy"
    },
    wh40k: {
        label: "WH40K",
        description: "Grimdark parchment + gold"
    },
    clean: {
        label: "Clean",
        description: "Purple accent, deep charcoal"
    },
    light: {
        label: "Light",
        description: "Classic light mode"
    },
    dark: {
        label: "Dark",
        description: "Classic dark mode"
    }
};
/**
 * Story 1.4.5 — Settings theme picker.
 *
 * Renders a grid of theme preview swatches. The active theme gets a primary-colored
 * border and checkmark. Switching themes is instant via ThemeContext.setTheme.
 */ function ThemePicker() {
    const { theme, setTheme, availableThemes, customThemes } = (0,ThemeContext/* useTheme */.DP)();
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: ThemePicker_css_container,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                        className: sectionTitle,
                        children: "Appearance"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: sectionDescription,
                        children: "Choose a theme. Your preference is saved locally and applied on next load."
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: grid,
                role: "radiogroup",
                "aria-label": "Theme selection",
                children: [
                    availableThemes.map((name)=>{
                        const meta = THEME_META[name];
                        const isActive = theme === name;
                        const swatchKey = name;
                        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                            role: "radio",
                            "aria-checked": isActive,
                            className: "".concat(themeButton).concat(isActive ? " ".concat(themeButtonActive) : ""),
                            onClick: ()=>setTheme(name),
                            title: meta.description,
                            children: [
                                isActive && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    className: activeCheckmark,
                                    "aria-hidden": "true",
                                    children: "✓"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                    className: previewSwatch[swatchKey],
                                    "aria-hidden": "true"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    className: themeName,
                                    children: meta.label
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    className: themeDescription,
                                    children: meta.description
                                })
                            ]
                        }, name);
                    }),
                    customThemes.map((custom)=>{
                        const id = "custom:".concat(custom.id);
                        const isActive = theme === id;
                        const swatchKey = custom.base;
                        var _custom_description;
                        return /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                            role: "radio",
                            "aria-checked": isActive,
                            className: "".concat(themeButton).concat(isActive ? " ".concat(themeButtonActive) : ""),
                            onClick: ()=>setTheme(id),
                            title: custom.description,
                            children: [
                                isActive && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    className: activeCheckmark,
                                    "aria-hidden": "true",
                                    children: "✓"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                    className: previewSwatch[swatchKey],
                                    style: {
                                        background: custom.tokens["color.background"],
                                        borderColor: custom.tokens["color.primary"]
                                    },
                                    "aria-hidden": "true"
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    className: themeName,
                                    children: custom.label
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                    className: themeDescription,
                                    children: (_custom_description = custom.description) !== null && _custom_description !== void 0 ? _custom_description : "Custom theme"
                                })
                            ]
                        }, id);
                    })
                ]
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/lib/hooks/useInputModeOverride.ts
var useInputModeOverride = __webpack_require__(27043);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsettings%2FInputModeSetting.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA62TTW6DMBCF95zCy0aKIyA_bZzDVAYPMKqxkW0IoerdK0xSERBKq0YskGaevzfzLG_a6nDc8pB8BoQItJXkF0YyCe0pIP5PBRpIHWrFSKplXaq-k_OKkYabF0rfoyN0sem20eoUfAWbgRh5YslNjoqR0NO0ctRiB7OTYvXTPwPmhZspCq9ItdRm2gvHtvFfbJNl6N0u2yekE3dj4s4TK21xOGpAcocNnP7vc_FLVVwIVPls5dB3E20EGEbiqiVWSxQTGYxU1HCBtZ2R3gYNTz9yo2slpoLXIdza2D7dSqNyYPqSg9ZRLjFXjEjInK8Zrm5ZXG39tZBwE-0tAW5hTRLdUltwoc-j8jhUVugGjI92DJlOhtftbjRGQv9FVTsNc393aSzTaW1pgxYTCd5H106iguUoS292lVGdZRacV4_A-4cjl78febdaE1QW3KKkHC918N6P3sgznubrAye-zIl7zjf5zDr8qwQAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsettings_2FInputModeSetting_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA62TTW6DMBCF95zCy0aKIyA_bZzDVAYPMKqxkW0IoerdK0xSERBKq0YskGaevzfzLG_a6nDc8pB8BoQItJXkF0YyCe0pIP5PBRpIHWrFSKplXaq_k_OKkYabF0rfoyN0sem20eoUfAWbgRh5YslNjoqR0NO0ctRiB7OTYvXTPwPmhZspCq9ItdRm2gvHtvFfbJNl6N0u2yekE3dj4s4TK21xOGpAcocNnP7vc_FLVVwIVPls5dB3E20EGEbiqiVWSxQTGYxU1HCBtZ2R3gYNTz9yo2slpoLXIdza2D7dSqNyYPqSg9ZRLjFXjEjInK8Zrm5ZXG39tZBwE_0tAW5hTRLdUltwoc_j8jhUVugGjI92DJlOhtftbjRGQv9FVTsNc393aSzTaW1pgxYTCd5H106iguUoS292lVGdZRacV4_A_4cjl78febdaE1QW3KKkHC918N6P3sgznubrAye_zIl7zjf5zDr8qwQAAA_22_7D = __webpack_require__(50557);
;// ./src/components/settings/InputModeSetting.css.ts


var InputModeSetting_css_container = 'xp693a0';
var optionButton = 'xp693a4';
var optionButtonActive = 'xp693a5';
var InputModeSetting_css_optionDescription = 'xp693a7';
var optionLabel = 'xp693a6';
var optionRow = 'xp693a3';
var InputModeSetting_css_sectionDescription = 'xp693a2';
var InputModeSetting_css_sectionTitle = 'xp693a1';
;// ./src/components/settings/InputModeSetting.tsx
/* __next_internal_client_entry_do_not_use__ InputModeSetting auto */ 
// +feature: settings-input-mode-override


const OPTIONS = [
    {
        value: "auto",
        label: "Auto-detect (recommended)",
        description: "Collapse the mobile keyboard row and toolbar when a mouse and physical keyboard are detected."
    },
    {
        value: "desktop",
        label: "Always compact",
        description: "Always hide the on-screen keyboard row and keep the toolbar collapsed by default."
    },
    {
        value: "touch",
        label: "Always touch-optimized",
        description: "Always show the on-screen keyboard row, even if a mouse is detected."
    }
];
/** Settings > Appearance — override for mouse+keyboard-on-mobile detection. */ function InputModeSetting() {
    const { inputModeOverride, setInputModeOverride } = (0,useInputModeOverride/* useInputModeOverride */.t)();
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: InputModeSetting_css_container,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                        className: InputModeSetting_css_sectionTitle,
                        children: "Terminal Input Mode"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: InputModeSetting_css_sectionDescription,
                        children: "Controls whether the compact terminal toolbar (no on-screen keyboard row) is used when a real mouse and keyboard are attached to a phone or tablet."
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: optionRow,
                role: "radiogroup",
                "aria-label": "Terminal input mode",
                children: OPTIONS.map((opt)=>{
                    const isActive = inputModeOverride === opt.value;
                    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                        role: "radio",
                        "aria-checked": isActive,
                        className: "".concat(optionButton).concat(isActive ? " ".concat(optionButtonActive) : ""),
                        onClick: ()=>setInputModeOverride(opt.value),
                        "data-testid": "input-mode-option-".concat(opt.value),
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: optionLabel,
                                children: opt.label
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                className: InputModeSetting_css_optionDescription,
                                children: opt.description
                            })
                        ]
                    }, opt.value);
                })
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/lib/config/terminalConfig.ts
var terminalConfig = __webpack_require__(81073);
;// ./src/components/settings/TerminalSettings.tsx
/* __next_internal_client_entry_do_not_use__ TerminalSettings auto */ 
// +feature: settings-terminal-config



const MIN_FONT_SIZE = 8;
const MAX_FONT_SIZE = 32;
const CURSOR_STYLES = [
    "block",
    "underline",
    "bar"
];
/** Settings > Appearance — font size and cursor for every terminal, applied live. */ function TerminalSettings() {
    const [values, setValues] = (0,react.useState)({
        fontSize: terminalConfig/* DEFAULT_TERMINAL_CONFIG */.Ai.fontSize,
        cursorStyle: terminalConfig/* DEFAULT_TERMINAL_CONFIG */.Ai.cursorStyle,
        cursorBlink: terminalConfig/* DEFAULT_TERMINAL_CONFIG */.Ai.cursorBlink
    });
    // Loaded after mount: localStorage is unavailable during SSR.
    (0,react.useEffect)(()=>{
        const { fontSize, cursorStyle, cursorBlink } = (0,terminalConfig/* loadTerminalConfig */.X6)();
        setValues({
            fontSize,
            cursorStyle,
            cursorBlink
        });
    }, []);
    const update = (patch)=>{
        setValues((prev)=>({
                ...prev,
                ...patch
            }));
        (0,terminalConfig/* saveTerminalConfig */.wo)(patch);
    };
    const onFontSize = (raw)=>{
        const n = Number(raw);
        if (!Number.isFinite(n)) return;
        update({
            fontSize: Math.max(MIN_FONT_SIZE, Math.min(MAX_FONT_SIZE, Math.round(n)))
        });
    };
    const reset = ()=>update({
            fontSize: terminalConfig/* DEFAULT_TERMINAL_CONFIG */.Ai.fontSize,
            cursorStyle: terminalConfig/* DEFAULT_TERMINAL_CONFIG */.Ai.cursorStyle,
            cursorBlink: terminalConfig/* DEFAULT_TERMINAL_CONFIG */.Ai.cursorBlink
        });
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: InputModeSetting_css_container,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                        className: InputModeSetting_css_sectionTitle,
                        children: "Terminal"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                        className: InputModeSetting_css_sectionDescription,
                        children: "Font size and cursor for all terminal sessions. Changes apply immediately and are stored in this browser."
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                className: optionLabel,
                children: [
                    "Font size (",
                    MIN_FONT_SIZE,
                    "–",
                    MAX_FONT_SIZE,
                    "px)",
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                        type: "number",
                        min: MIN_FONT_SIZE,
                        max: MAX_FONT_SIZE,
                        value: values.fontSize,
                        onChange: (e)=>onFontSize(e.target.value),
                        "data-testid": "terminal-font-size"
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: optionRow,
                role: "radiogroup",
                "aria-label": "Cursor style",
                children: CURSOR_STYLES.map((style)=>{
                    const isActive = values.cursorStyle === style;
                    return /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        role: "radio",
                        "aria-checked": isActive,
                        className: "".concat(optionButton).concat(isActive ? " ".concat(optionButtonActive) : ""),
                        onClick: ()=>update({
                                cursorStyle: style
                            }),
                        "data-testid": "terminal-cursor-".concat(style),
                        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                            className: optionLabel,
                            children: [
                                style[0].toUpperCase() + style.slice(1),
                                " cursor"
                            ]
                        })
                    }, style);
                })
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("label", {
                className: optionLabel,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("input", {
                        type: "checkbox",
                        checked: values.cursorBlink,
                        onChange: (e)=>update({
                                cursorBlink: e.target.checked
                            }),
                        "data-testid": "terminal-cursor-blink"
                    }),
                    " ",
                    "Blinking cursor"
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                type: "button",
                className: optionButton,
                onClick: reset,
                "data-testid": "terminal-settings-reset",
                children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                    className: optionLabel,
                    children: "Reset to defaults"
                })
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@monaco-editor+react@4.7.0+40630488a76e255a/node_modules/@monaco-editor/react/dist/index.mjs + 11 modules
var react_dist = __webpack_require__(83037);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fapp%2Fconfig%2Fconfig.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA9VZ3ZKjKhC-n6fg5lRtqo4pcfLjMDf7JltEScKOgouYnzk1735K1AgIajIzF3uVKqSbr5vuj-7O8lcUnTCOQvDfEwApLYsMXxHYZ-Ty-gTAARcIRGFxeX36eFq2e6Hae6apPCIQrdVXoESC8igoe0Mg1PdHan-OxYGyYMel5DkCsBPjTAYlfScIwFhbOhN6OEoENmFYryU84wKBExY_guAXfCHvkXgPF_oxz24TFK6UCpJIyhmqNVU5uxm3Nm1buZXgjB5YQCXJSwQSwiQRNw0t6gKnKWWH3jJJLjJQgghkZC_rtR0XKREIwOICSp7R1LKILPpdgcAprUoEVo2-HU7eDoJXLA2c3tgq2aQSZf2t4LRDafiTcZHjbNSlAEiBWUkbf9nHgnAZlf92GPslw4voyE9EKF9OwY6NS1zPkskXXgP-LHwhpCN2KdRA3AGfGZKbRmbkpIuxf6v226ljZsXKjND4JoIArPfWUPcZPyNwpGlK2C32-g8ky2hR0rL-dD5SSYKywAmpo-EscKGrf1HqvdBNaJOpj2fYB638iYoL2LQhbybC87xEgFt_eES9DXuc0-yKQM4ZV-7Qge8U8D6nN8UFwGiY2D0XuKGEA58966b5maCVfIwKIDSiLBkj4DMXabATBL8hoH6CekUXT01fKFeshuz97GDvtZ56nzbWw3gjjnew4QS3wbXBZOnDTEZ0p9f8f3spdKdFDqf1FG3kROzMie4ap8Bd_b4yKWlv8kv_4eB-Gn9XpaT7a5BwJgmTCKhcCnZEnklDRp7H0xmP_XHHBseXlQd0itiMq7LR_B6pj-y9b2a-rBz1wARtRAs9a-rLT3F5JPOrhR5MZgKnLKOMBBN1TY9doz13ds5lgInwPPnDs1rMoZDKuOz877Ba8fQ4g82izt5wprOO5IVGEN_-4LhKkR4ZN7Mi1n08T3sL7UYY8zxSDHjEdWFjPGIp_NO6-BIcW2EYtj7u7A-uCOBKcl1MzOgsmkJJYiG93YXutwdft0c9KVCGSxkkR5qlep3bqWGcEXP__LfTqlhKP1lboOQ4rfcbqz4OHJWf81Ue4eaJKvrkC7qRpmu6EDsPkjsK7ebTTVubmYm2ncU5xHDsZWCrq6gZS7DGpep9VPG75yJHoCoKIhJcqtvJiJREKIcrO8PlujnEqiJi8_W7PtzOzw0HuHCg2Fgo7MSxthst0_vgkp8j45Lbm18ZF91Q_az7gqGVke6y-qFRhc1WXXzeO5Xp-ouaZdtB0ya0x1DwoYGPVR_B6J7wfSxGvUGvwfBMryYjVVOxuoPhpunIHx93sISvnjBKrS7jvmukAddWQV4_oY7xYzSvUJo3o7tnKAF9varTsHtbWOgeZw1AmO0r3Pgf1rzn65QkXOAGSvP-O-JM16phGSioWEpEXakbIls_kMg3XWmW5DUjCFCJM5oYKuNv59iXv5JjV7HNsfhLm35N8W5AuysH7a7HynLT44k_TCrvTHhkzgrTyZa_dXGdlgh0yWlEkK1THwoNR6HhMl4Lkk-2hP4chvN7Rl-NeIurcFmDAeFy24DSzNh_yoxqworT7Ib_MzZ4xlkTSRPP5PnBv0GegvyuShse7yoSppO0ieXIClL6pXWENkn8mZOUYlAmghAGMEvBD418tpu4uCzU2R2UplIdu5QPbTdsd7f6YBj-86oW9F496ujS1a3XqzN744aa9eP33uMp62wMbaFzK3S70-4R_Xj6-B_rg2wNnB0AAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fapp_2Fconfig_2Fconfig_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA9VZ3ZKjKhC_n6fg5lRtqo4pcfLjMDf7JltEScKOgouYnzk1735K1AgIajIzF3uVKqSbr5vuj_7O8lcUnTCOQvDfEwApLYsMXxHYZ_Ty_gTAARcIRGFxeX36eFq2e6Hae6apPCIQrdVXoESC8igoe0Mg1PdHan_OxYGyYMel5DkCsBPjTAYlfScIwFhbOhN6OEoENmFYryU84wKBExY_guAXfCHvkXgPF_oxz24TFK6UCpJIyhmqNVU5uxm3Nm1buZXgjB5YQCXJSwQSwiQRNw0t6gKnKWWH3jJJLjJQgghkZC_rtR0XKREIwOICSp7R1LKILPpdgcAprUoEVo2_HU7eDoJXLA2c3tgq2aQSZf2t4LRDafiTcZHjbNSlAEiBWUkbf9nHgnAZlf92GPslw4voyE9EKF9OwY6NS1zPkskXXgP_LHwhpCN2KdRA3AGfGZKbRmbkpIuxf6v226ljZsXKjND4JoIArPfWUPcZPyNwpGlK2C32_g8ky2hR0rL_dD5SSYKywAmpo_EscKGrf1HqvdBNaJOpj2fYB638iYoL2LQhbybC87xEgFt_eES9DXuc0_yKQM4ZV_7Qge8U8D6nN8UFwGiY2D0XuKGEA58966b5maCVfIwKIDSiLBkj4DMXabATBL8hoH6CekUXT01fKFeshuz97GDvtZ56nzbWw3gjjnew4QS3wbXBZOnDTEZ0p9f8f3spdKdFDqf1FG3kROzMie4ap8Bd_b4yKWlv8kv_4eB_Gn9XpaT7a5BwJgmTCKhcCnZEnklDRp7H0xmP_XHHBseXlQd0itiMq7LR_B6pj_y9b2a_rBz1wARtRAs9a_rLT3F5JPOrhR5MZgKnLKOMBBN1TY9doz13ds5lgInwPPnDs1rMoZDKuOz877Ba8fQ4g82izt5wprOO5IVGEN_4LhKkR4ZN7Mi1n08T3sL7UYY8zxSDHjEdWFjPGIp_NO6_BIcW2EYtj7u7A_uCOBKcl1MzOgsmkJJYiG93YXutwdft0c9KVCGSxkkR5qlep3bqWGcEXP_LfTqlhKP1lboOQ4rfcbqz4OHJWf81Ue4eaJKvrkC7qRpmu6EDsPkjsK7ebTTVubmYm2ncU5xHDsZWCrq6gZS7DGpep9VPG75yJHoCoKIhJcqtvJiJREKIcrO8PlujnEqiJi8_W7PtzOzw0HuHCg2Fgo7MSxthst0_vgkp8j45Lbm18ZF91Q_az7gqGVke6y_qFRhc1WXXzeO5Xp_ouaZdtB0ya0x1DwoYGPVR_B6J7wfSxGvUGvwfBMryYjVVOxuoPhpunIHx93sISvnjBKrS7jvmukAddWQV4_oY7xYzSvUJo3o7tnKAF9varTsHtbWOgeZw1AmO0r3Pgf1rzn65QkXOAGSvP_O_JM16phGSioWEpEXakbIls_kMg3XWmW5DUjCFCJM5oYKuNv59iXv5JjV7HNsfhLm35N8W5AuysH7a7HynLT44k_TCrvTHhkzgrTyZa_dXGdlgh0yWlEkK1THwoNR6HhMl4Lkk_2hP4chvN7Rl_NeIurcFmDAeFy24DSzNh_yoxqworT7Ib_MzZ4xlkTSRPP5PnBv0GegvyuShse7yoSppO0ieXIClL6pXWENkn8mZOUYlAmghAGMEvBD418tpu4uCzU2R2UplIdu5QPbTdsd7f6YBj_86oW9F496ujS1a3XqzN744aa9eP33uMp62wMbaFzK3S70_4R_Xj6_B_rg2wNnB0AAA_22_7D = __webpack_require__(34943);
;// ./src/app/config/config.css.ts


var buttonGroup = '_22vaa2j';
var config_css_content = '_22vaa20';
var editor = '_22vaa2f';
var editorHeader = '_22vaa2g';
var editorTitle = '_22vaa2h';
var emptyState = '_22vaa2k';
var fileButton = '_22vaa24';
var fileButtonModified = '_22vaa26';
var fileButtonSelected = '_22vaa25';
var fileIcon = '_22vaa27';
var fileList = '_22vaa21';
var fileListErrorMessage = '_22vaa2c';
var fileListErrorState = '_22vaa2b';
var fileListItems = '_22vaa23';
var fileModifiedDot = '_22vaa29';
var fileName = '_22vaa28';
var fileShortcut = '_22vaa2a';
var hostnameItem = '_22vaa21h';
var hostnameText = '_22vaa21i';
var hostnamesList = '_22vaa21g';
var modifiedIndicator = '_22vaa2i';
var networkCard = '_22vaa210';
var networkCopyBtn = '_22vaa215';
var networkDisabledNote = '_22vaa217';
var networkLabel = '_22vaa212';
var networkLink = '_22vaa216';
var networkRow = '_22vaa211';
var networkSection = '_22vaa2z';
var networkValue = '_22vaa213';
var networkValueText = '_22vaa214';
var retryButton = '_22vaa2d';
var config_css_sectionTitle = '_22vaa22';
var securityActions = '_22vaa21d';
var securityCard = '_22vaa219';
var securityError = '_22vaa21e';
var securityLabel = '_22vaa21b';
var securityRow = '_22vaa21a';
var securitySection = '_22vaa218';
var securitySuccess = '_22vaa21f';
var shortcutItem = '_22vaa2y';
var shortcutsHelp = '_22vaa2w';
var shortcutsTitle = '_22vaa2x';
var statusEnabled = '_22vaa21c';
var unsavedCount = '_22vaa2e';
var validationBadgeError = '_22vaa2m';
var validationBadgeValid = '_22vaa2l';
var validationError = '_22vaa2r';
var validationErrorLocation = '_22vaa2u';
var validationErrorMessage = '_22vaa2v';
var validationErrorSeverity = '_22vaa2s';
var validationPanel = '_22vaa2n';
var validationPanelContent = '_22vaa2q';
var validationPanelHeader = '_22vaa2o';
var validationPanelTitle = '_22vaa2p';
var validationWarningSeverity = '_22vaa2t';
// EXTERNAL MODULE: ./src/lib/contexts/AuthContext.tsx
var AuthContext = __webpack_require__(64792);
// EXTERNAL MODULE: ./src/lib/auth/passkey.ts
var passkey = __webpack_require__(15681);
;// ./src/app/config/ConfigPageContent.tsx
/* __next_internal_client_entry_do_not_use__ ConfigPageContent auto */ 









// Helper function to determine Monaco editor language from filename
function getLanguageFromFilename(filename) {
    if (filename.endsWith('.json')) return 'json';
    if (filename.endsWith('.md')) return 'markdown';
    return 'plaintext';
}
// Helper function to get file icon based on filename
function getFileIcon(filename) {
    if (filename.endsWith('.json')) return '📋';
    if (filename.endsWith('.md')) return '📝';
    if (filename === 'CLAUDE.md') return '🤖';
    return '📄';
}
// How long to wait for listClaudeConfigs before giving up and surfacing an
// error instead of leaving the file list stuck on "Loading..." forever. A
// hung/unresponsive server never rejects the RPC promise on its own — without
// this the fetch just never settles and `loading` stays true indefinitely.
const CONFIG_LOAD_TIMEOUT_MS = 15000;
function ConfigPageContent() {
    const [configs, setConfigs] = (0,react.useState)([]);
    const [selectedConfig, setSelectedConfig] = (0,react.useState)(null);
    const [content, setContent] = (0,react.useState)("");
    const [originalContent, setOriginalContent] = (0,react.useState)("");
    const [loading, setLoading] = (0,react.useState)(true);
    const [saving, setSaving] = (0,react.useState)(false);
    const [error, setError] = (0,react.useState)(null);
    const [successMessage, setSuccessMessage] = (0,react.useState)(null);
    const [validationErrors, setValidationErrors] = (0,react.useState)([]);
    const [isValidating, setIsValidating] = (0,react.useState)(false);
    // Per-file state tracking for multi-file navigation
    const [fileStates, setFileStates] = (0,react.useState)({});
    const [serverInfo, setServerInfo] = (0,react.useState)(null);
    const [copiedField, setCopiedField] = (0,react.useState)(null);
    const clientRef = (0,react.useRef)(null);
    const editorRef = (0,react.useRef)(null);
    const monacoRef = (0,react.useRef)(null);
    var _selectedConfig_name_endsWith;
    // Check if current file requires validation (JSON files)
    const requiresValidation = (_selectedConfig_name_endsWith = selectedConfig === null || selectedConfig === void 0 ? void 0 : selectedConfig.name.endsWith('.json')) !== null && _selectedConfig_name_endsWith !== void 0 ? _selectedConfig_name_endsWith : false;
    const hasValidationErrors = validationErrors.some((e)=>e.severity === 'error');
    const canSave = !hasValidationErrors || !requiresValidation;
    // Initialize ConnectRPC client
    (0,react.useEffect)(()=>{
        const transport = (0,connect_transport/* createConnectTransport */.w)({
            baseUrl: (0,config/* getApiBaseUrl */.h)()
        });
        clientRef.current = (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, transport);
    }, []);
    // Load configs on mount
    (0,react.useEffect)(()=>{
        loadConfigs();
    }, []);
    // Load server info on mount
    (0,react.useEffect)(()=>{
        fetch('/api/server-info').then((r)=>r.json()).then((data)=>setServerInfo(data)).catch(()=>{});
    }, []);
    // Validate JSON content and update Monaco markers
    const validateContent = (0,react.useCallback)((value, filename)=>{
        if (!filename.endsWith('.json')) {
            setValidationErrors([]);
            return;
        }
        setIsValidating(true);
        const errors = [];
        try {
            JSON.parse(value);
        // JSON is valid - clear errors
        } catch (e) {
            if (e instanceof SyntaxError) {
                // Extract line/column from JSON parse error message
                const match = e.message.match(/at position (\d+)/);
                let line = 1;
                let column = 1;
                if (match) {
                    const position = parseInt(match[1], 10);
                    // Convert position to line/column
                    const lines = value.substring(0, position).split('\n');
                    line = lines.length;
                    column = lines[lines.length - 1].length + 1;
                }
                errors.push({
                    line,
                    column,
                    message: e.message.replace(/^JSON\.parse: /, ''),
                    severity: 'error'
                });
            }
        }
        setValidationErrors(errors);
        setIsValidating(false);
        // Update Monaco editor markers
        if (monacoRef.current && editorRef.current) {
            const model = editorRef.current.getModel();
            if (model) {
                const markers = errors.map((err)=>({
                        severity: err.severity === 'error' ? monacoRef.current.MarkerSeverity.Error : monacoRef.current.MarkerSeverity.Warning,
                        startLineNumber: err.line,
                        startColumn: err.column,
                        endLineNumber: err.line,
                        endColumn: err.column + 1,
                        message: err.message
                    }));
                monacoRef.current.editor.setModelMarkers(model, 'json-validator', markers);
            }
        }
    }, []);
    // Editor mount handler
    const handleEditorMount = (editor, monaco)=>{
        editorRef.current = editor;
        monacoRef.current = monaco;
        // Enable JSON validation in Monaco
        monaco.languages.json.jsonDefaults.setDiagnosticsOptions({
            validate: true,
            allowComments: false,
            schemas: [],
            enableSchemaRequest: false
        });
        // Validate initial content if JSON file
        if ((selectedConfig === null || selectedConfig === void 0 ? void 0 : selectedConfig.name.endsWith('.json')) && content) {
            validateContent(content, selectedConfig.name);
        }
    };
    // Validate on content change (debounced)
    (0,react.useEffect)(()=>{
        if (!selectedConfig) return;
        const timeoutId = setTimeout(()=>{
            validateContent(content, selectedConfig.name);
        }, 300); // 300ms debounce
        return ()=>clearTimeout(timeoutId);
    }, [
        content,
        selectedConfig,
        validateContent
    ]);
    // Clear validation errors when switching files
    (0,react.useEffect)(()=>{
        setValidationErrors([]);
    }, [
        selectedConfig === null || selectedConfig === void 0 ? void 0 : selectedConfig.name
    ]);
    const loadConfigs = async ()=>{
        if (!clientRef.current) return;
        try {
            setLoading(true);
            setError(null);
            const response = await Promise.race([
                clientRef.current.listClaudeConfigs({}),
                new Promise((_, reject)=>setTimeout(()=>reject(new Error("Timed out waiting for the server to respond.")), CONFIG_LOAD_TIMEOUT_MS))
            ]);
            setConfigs(response.configs);
        } catch (err) {
            setError("Failed to load configs: ".concat(err instanceof Error ? err.message : String(err)));
        } finally{
            setLoading(false);
        }
    };
    const loadConfig = (0,react.useCallback)(async (filename)=>{
        if (!clientRef.current) return;
        // Save current file state before switching
        if (selectedConfig && content !== undefined) {
            setFileStates((prev)=>({
                    ...prev,
                    [selectedConfig.name]: {
                        content,
                        originalContent
                    }
                }));
        }
        // Check if we have cached state for this file
        const cachedState = fileStates[filename];
        if (cachedState) {
            // Find the config in our list
            const config = configs.find((c)=>c.name === filename);
            if (config) {
                setSelectedConfig(config);
                setContent(cachedState.content);
                setOriginalContent(cachedState.originalContent);
                return;
            }
        }
        try {
            setError(null);
            const response = await clientRef.current.getClaudeConfig({
                filename
            });
            if (response.config) {
                setSelectedConfig(response.config);
                setContent(response.config.content);
                setOriginalContent(response.config.content);
                // Cache the initial state
                setFileStates((prev)=>({
                        ...prev,
                        [filename]: {
                            content: response.config.content,
                            originalContent: response.config.content
                        }
                    }));
            }
        } catch (err) {
            setError("Failed to load config: ".concat(err));
        }
    }, [
        selectedConfig,
        content,
        originalContent,
        fileStates,
        configs
    ]);
    const saveConfig = (0,react.useCallback)(async ()=>{
        if (!selectedConfig || !clientRef.current) return;
        try {
            setSaving(true);
            setError(null);
            setSuccessMessage(null);
            await clientRef.current.updateClaudeConfig({
                filename: selectedConfig.name,
                content: content
            });
            setOriginalContent(content);
            // Update cached file state to reflect saved content
            setFileStates((prev)=>({
                    ...prev,
                    [selectedConfig.name]: {
                        content,
                        originalContent: content
                    }
                }));
            setSuccessMessage("✓ Saved ".concat(selectedConfig.name));
            // Auto-clear success message after 3 seconds
            setTimeout(()=>setSuccessMessage(null), 3000);
        } catch (err) {
            setError("Failed to save config: ".concat(err));
        } finally{
            setSaving(false);
        }
    }, [
        selectedConfig,
        content
    ]);
    const hasUnsavedChanges = content !== originalContent;
    // Check if a specific file has unsaved changes
    const fileHasUnsavedChanges = (0,react.useCallback)((filename)=>{
        // If it's the current file, use the live state
        if ((selectedConfig === null || selectedConfig === void 0 ? void 0 : selectedConfig.name) === filename) {
            return content !== originalContent;
        }
        // Otherwise check cached state
        const state = fileStates[filename];
        return state ? state.content !== state.originalContent : false;
    }, [
        selectedConfig,
        content,
        originalContent,
        fileStates
    ]);
    // Count total unsaved files
    const unsavedFilesCount = configs.filter((c)=>fileHasUnsavedChanges(c.name)).length;
    // Passkey management
    const { authEnabled, authenticated, hasCredentials, refresh: refreshAuth } = (0,AuthContext/* useAuth */.A)();
    const [passkeyStatus, setPasskeyStatus] = (0,react.useState)("idle");
    const [passkeyMsg, setPasskeyMsg] = (0,react.useState)("");
    const handleRegisterPasskey = async ()=>{
        setPasskeyStatus("working");
        setPasskeyMsg("");
        try {
            await (0,passkey/* registerPasskey */.eA)();
            await refreshAuth();
            setPasskeyStatus("success");
            setPasskeyMsg("Passkey registered successfully.");
            setTimeout(()=>{
                setPasskeyStatus("idle");
                setPasskeyMsg("");
            }, 4000);
        } catch (e) {
            setPasskeyStatus("error");
            setPasskeyMsg(e instanceof Error ? e.message : String(e));
        }
    };
    const handleRevokeAll = async ()=>{
        setPasskeyStatus("working");
        setPasskeyMsg("");
        try {
            await (0,passkey/* logout */.ri)();
            await refreshAuth();
            setPasskeyStatus("success");
            setPasskeyMsg("All sessions revoked.");
            setTimeout(()=>{
                setPasskeyStatus("idle");
                setPasskeyMsg("");
            }, 4000);
        } catch (e) {
            setPasskeyStatus("error");
            setPasskeyMsg(e instanceof Error ? e.message : String(e));
        }
    };
    const copyToClipboard = (text, field)=>{
        navigator.clipboard.writeText(text).then(()=>{
            setCopiedField(field);
            setTimeout(()=>setCopiedField(null), 2000);
        });
    };
    // Handle keyboard shortcuts (Ctrl+S to save, Ctrl+1-9 for file switching)
    (0,react.useEffect)(()=>{
        const handleKeyDown = (e)=>{
            // Ctrl+S or Cmd+S to save
            if ((e.ctrlKey || e.metaKey) && e.key === 's') {
                e.preventDefault();
                if (hasUnsavedChanges && !saving && canSave) {
                    saveConfig();
                }
                return;
            }
            // Ctrl+1-9 or Cmd+1-9 for quick file switching
            if ((e.ctrlKey || e.metaKey) && e.key >= '1' && e.key <= '9') {
                e.preventDefault();
                const index = parseInt(e.key, 10) - 1;
                if (index < configs.length) {
                    loadConfig(configs[index].name);
                }
                return;
            }
            // Ctrl+[ or Cmd+[ for previous file
            if ((e.ctrlKey || e.metaKey) && e.key === '[') {
                e.preventDefault();
                if (selectedConfig && configs.length > 0) {
                    const currentIndex = configs.findIndex((c)=>c.name === selectedConfig.name);
                    const prevIndex = currentIndex > 0 ? currentIndex - 1 : configs.length - 1;
                    loadConfig(configs[prevIndex].name);
                }
                return;
            }
            // Ctrl+] or Cmd+] for next file
            if ((e.ctrlKey || e.metaKey) && e.key === ']') {
                e.preventDefault();
                if (selectedConfig && configs.length > 0) {
                    const currentIndex = configs.findIndex((c)=>c.name === selectedConfig.name);
                    const nextIndex = currentIndex < configs.length - 1 ? currentIndex + 1 : 0;
                    loadConfig(configs[nextIndex].name);
                }
                return;
            }
        };
        window.addEventListener('keydown', handleKeyDown);
        return ()=>window.removeEventListener('keydown', handleKeyDown);
    }, [
        hasUnsavedChanges,
        saving,
        canSave,
        content,
        selectedConfig,
        configs,
        loadConfig,
        saveConfig
    ]);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        children: [
            error && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: "alert alert-error",
                children: error
            }),
            successMessage && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                className: "alert alert-success",
                children: successMessage
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: config_css_content,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: fileList,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("h2", {
                                className: config_css_sectionTitle,
                                children: [
                                    "Config Files",
                                    unsavedFilesCount > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                        className: unsavedCount,
                                        children: [
                                            unsavedFilesCount,
                                            " unsaved"
                                        ]
                                    })
                                ]
                            }),
                            loading ? /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                children: "Loading..."
                            }) : error && configs.length === 0 ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: fileListErrorState,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                        className: fileListErrorMessage,
                                        children: error
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        className: retryButton,
                                        onClick: loadConfigs,
                                        children: "Retry"
                                    })
                                ]
                            }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                className: fileListItems,
                                children: configs.map((config, index)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("button", {
                                        onClick: ()=>loadConfig(config.name),
                                        className: "".concat(fileButton, " ").concat((selectedConfig === null || selectedConfig === void 0 ? void 0 : selectedConfig.name) === config.name ? fileButtonSelected : "", " ").concat(fileHasUnsavedChanges(config.name) ? fileButtonModified : ""),
                                        title: "".concat(config.name).concat(index < 9 ? " (Ctrl+".concat(index + 1, ")") : ''),
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: fileIcon,
                                                children: getFileIcon(config.name)
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: fileName,
                                                children: config.name
                                            }),
                                            fileHasUnsavedChanges(config.name) && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: fileModifiedDot,
                                                children: "●"
                                            }),
                                            index < 9 && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                className: fileShortcut,
                                                children: index + 1
                                            })
                                        ]
                                    }, config.name))
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: shortcutsHelp,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                        className: shortcutsTitle,
                                        children: "Shortcuts"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        className: shortcutItem,
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("kbd", {
                                                children: "Ctrl+S"
                                            }),
                                            " Save"
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        className: shortcutItem,
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("kbd", {
                                                children: "Ctrl+1-9"
                                            }),
                                            " Switch file"
                                        ]
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        className: shortcutItem,
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("kbd", {
                                                children: "Ctrl+[/]"
                                            }),
                                            " Prev/Next"
                                        ]
                                    })
                                ]
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: editor,
                        children: selectedConfig ? /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: editorHeader,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("h2", {
                                            className: editorTitle,
                                            children: [
                                                selectedConfig.name,
                                                hasUnsavedChanges && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                    className: modifiedIndicator,
                                                    children: "[modified]"
                                                })
                                            ]
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                            className: buttonGroup,
                                            children: [
                                                requiresValidation && /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                    className: hasValidationErrors ? validationBadgeError : validationBadgeValid,
                                                    children: isValidating ? '⏳ Validating...' : hasValidationErrors ? "❌ ".concat(validationErrors.length, " error(s)") : '✓ Valid JSON'
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                    onClick: saveConfig,
                                                    disabled: !hasUnsavedChanges || saving || !canSave,
                                                    className: "btn btn-primary",
                                                    title: !canSave ? 'Fix validation errors before saving' : '',
                                                    children: saving ? "Saving..." : "Save"
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                    onClick: ()=>setContent(originalContent),
                                                    disabled: !hasUnsavedChanges,
                                                    className: "btn btn-danger",
                                                    children: "Discard"
                                                })
                                            ]
                                        })
                                    ]
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)(react_dist/* default */.Ay, {
                                    height: validationErrors.length > 0 ? "500px" : "600px",
                                    language: getLanguageFromFilename(selectedConfig.name),
                                    theme: "vs-dark",
                                    value: content,
                                    onChange: (value)=>setContent(value || ""),
                                    onMount: handleEditorMount,
                                    options: {
                                        readOnly: saving,
                                        minimap: {
                                            enabled: true
                                        },
                                        fontSize: 14,
                                        lineNumbers: 'on',
                                        folding: true,
                                        tabSize: 2,
                                        automaticLayout: true,
                                        scrollBeyondLastLine: false,
                                        wordWrap: 'on',
                                        formatOnPaste: true,
                                        formatOnType: true,
                                        bracketPairColorization: {
                                            enabled: true
                                        }
                                    }
                                }),
                                validationErrors.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: validationPanel,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                            className: validationPanelHeader,
                                            children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                className: validationPanelTitle,
                                                children: [
                                                    "⚠️ Validation Errors (",
                                                    validationErrors.length,
                                                    ")"
                                                ]
                                            })
                                        }),
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                            className: validationPanelContent,
                                            children: validationErrors.map((err, index)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                    className: "".concat(validationError, " ").concat(err.severity === 'error' ? validationErrorSeverity : validationWarningSeverity),
                                                    onClick: ()=>{
                                                        // Jump to error location in editor
                                                        if (editorRef.current) {
                                                            editorRef.current.setPosition({
                                                                lineNumber: err.line,
                                                                column: err.column
                                                            });
                                                            editorRef.current.focus();
                                                            editorRef.current.revealLineInCenter(err.line);
                                                        }
                                                    },
                                                    children: [
                                                        /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                                            className: validationErrorLocation,
                                                            children: [
                                                                "Line ",
                                                                err.line,
                                                                ", Col ",
                                                                err.column,
                                                                ":"
                                                            ]
                                                        }),
                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                            className: validationErrorMessage,
                                                            children: err.message
                                                        })
                                                    ]
                                                }, index))
                                        })
                                    ]
                                })
                            ]
                        }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                            className: emptyState,
                            children: "Select a config file to edit"
                        })
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: networkSection,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                        className: config_css_sectionTitle,
                        children: "\uD83C\uDF10 Network & Remote Access"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: networkCard,
                        children: serverInfo ? /*#__PURE__*/ (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: networkRow,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: networkLabel,
                                            children: "HTTPS URL"
                                        }),
                                        serverInfo.https_url ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                            className: networkValue,
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                    className: networkValueText,
                                                    title: serverInfo.https_url,
                                                    children: serverInfo.https_url
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("a", {
                                                    href: serverInfo.https_url,
                                                    target: "_blank",
                                                    rel: "noreferrer",
                                                    className: networkLink,
                                                    children: "Open"
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                    className: networkCopyBtn,
                                                    onClick: ()=>copyToClipboard(serverInfo.https_url, 'httpsUrl'),
                                                    children: copiedField === 'httpsUrl' ? 'Copied!' : 'Copy'
                                                })
                                            ]
                                        }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: networkDisabledNote,
                                            children: "Remote access not enabled (start with --remote-access)"
                                        })
                                    ]
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: networkRow,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: networkLabel,
                                            children: "CA Certificate Path"
                                        }),
                                        serverInfo.tls_enabled && serverInfo.ca_pem_path ? /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                            className: networkValue,
                                            children: [
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                    className: networkValueText,
                                                    title: serverInfo.ca_pem_path,
                                                    children: serverInfo.ca_pem_path
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                    className: networkCopyBtn,
                                                    onClick: ()=>copyToClipboard(serverInfo.ca_pem_path, 'caPath'),
                                                    children: copiedField === 'caPath' ? 'Copied!' : 'Copy'
                                                }),
                                                /*#__PURE__*/ (0,jsx_runtime.jsx)("a", {
                                                    href: "/auth/ca.pem",
                                                    download: "stapler-squad-ca.pem",
                                                    className: networkLink,
                                                    children: "Download"
                                                })
                                            ]
                                        }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: networkDisabledNote,
                                            children: "TLS not active"
                                        })
                                    ]
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: networkRow,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: networkLabel,
                                            children: "Detected Hostnames"
                                        }),
                                        serverInfo.hostnames && serverInfo.hostnames.length > 0 ? /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                            className: hostnamesList,
                                            children: serverInfo.hostnames.map((hn, idx)=>/*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                    className: hostnameItem,
                                                    children: [
                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                            className: hostnameText,
                                                            children: hn
                                                        }),
                                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                            className: networkCopyBtn,
                                                            onClick: ()=>copyToClipboard(hn, "hostname-".concat(idx)),
                                                            children: copiedField === "hostname-".concat(idx) ? 'Copied!' : 'Copy'
                                                        })
                                                    ]
                                                }, idx))
                                        }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: networkDisabledNote,
                                            children: "No LAN hostnames detected"
                                        })
                                    ]
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                    className: networkRow,
                                    children: [
                                        /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: networkLabel,
                                            children: "Available Programs"
                                        }),
                                        serverInfo.programs && serverInfo.programs.length > 0 ? /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                            className: hostnamesList,
                                            children: serverInfo.programs.map((p, idx)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                                    className: hostnameItem,
                                                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                                        className: hostnameText,
                                                        children: p
                                                    })
                                                }, idx))
                                        }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                            className: networkDisabledNote,
                                            children: "No CLI programs detected"
                                        })
                                    ]
                                })
                            ]
                        }) : /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                            className: networkDisabledNote,
                            children: "Loading…"
                        })
                    })
                ]
            }),
            authEnabled && /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: securitySection,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("h2", {
                        className: config_css_sectionTitle,
                        children: "\uD83D\uDD10 Passkey Security"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: securityCard,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: securityRow,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: securityLabel,
                                        children: "Authentication"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: statusEnabled,
                                        children: "Active"
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: securityRow,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: securityLabel,
                                        children: "Passkeys registered"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        children: hasCredentials ? "✓ Yes" : "None"
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: securityActions,
                                children: [
                                    authenticated && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        onClick: handleRegisterPasskey,
                                        disabled: passkeyStatus === "working",
                                        className: "btn btn-primary",
                                        children: passkeyStatus === "working" ? "Working…" : "Register New Passkey"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                        onClick: handleRevokeAll,
                                        disabled: passkeyStatus === "working" || !authenticated,
                                        className: "btn btn-danger",
                                        children: "Sign Out All Sessions"
                                    })
                                ]
                            }),
                            passkeyStatus === "error" && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                className: securityError,
                                children: passkeyMsg
                            }),
                            passkeyStatus === "success" && /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                                className: securitySuccess,
                                children: passkeyMsg
                            })
                        ]
                    })
                ]
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/lib/shortcuts/shortcutRegistry.ts
var shortcutRegistry = __webpack_require__(8569);
// EXTERNAL MODULE: ./src/lib/shortcuts/leaderConfig.ts
var leaderConfig = __webpack_require__(78475);
// EXTERNAL MODULE: ./src/lib/shortcuts/commandBarController.ts
var commandBarController = __webpack_require__(4942);
// EXTERNAL MODULE: ./src/components/ui/Kbd.tsx + 1 modules
var Kbd = __webpack_require__(71048);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fapp%2Fsettings%2Fsettings.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA8VW226jMBB9z1dYlVbaPDgyZJM0rvZLVqvKwRNwC7ZlTEi76r-vMCkXA2nTVO1TlGE8c-b4zIHF_epBr0VJ0L8ZQhk74lJwm1C0XRN9vHMxEwtJEUGssKqKaMa5kDFFoYEMBYuVgayKJyDixFIUEPKj-q8OYPapKvETbc42sSNFieAc5N3sZbY4oQgcikilylB0YOYnxvfBFp5D80zm1fG9khbn4hlop68LlqfmG0Ja1HinrFVZm9y2Cl0rLnKdsieK9im4aVkqYomFhSynKAJpwVThhyK3Yv-EIyUtSEtRrlkEeAe2hGoEhHbKcDBtQ31EuUoF98aA-Qi60MO2HMcWM00R6Sb-conNfZDFOly5O-kyU9PVh7HczQfMeRmxyxi9i9A9snC0mEOkDLNCSYqkklA9KBNhATuGqmBpmHb5hslc1KleLznvTUWTSibntdCmr76ThB2LHmOjCsnb-Wsp-P_b226k4RjRzIC0rk1h8qqPVuJVd55QcFDv5LupXF1I5R_OLMO5ZRZ-37DIigPc_J0-nc0H8-HJxLbN-tMr3noa-Jj9bMf3rvrFXBiIasYjlRaZbDbS317mqlzkByfgk4bAaMpyi6NEpHys-qvS_Do9s9h1OcJWadqsyKUjBx6-yJW-ZtGS6UULWrdxut8rk1FUaA0mYrmbOwVrwTjLOXkAWdWTeQtEBu8BftF7oDP9iaTK3SiqPa6tCr4nfYUjkYEj9Z9v5l13Oq_Ik8AM46LIByDqSiN-9W5jgo4xTSO-7WIZnVn0qu6_g_bs3Nuwg60z8SC5kBxMKvon4k-xo-RDVYaLIt7YcXbNjofX7PjA9Zovw2s-ysgi9Cl4GKfy7Y_DCTfpCHW5cVLtGfbju1x10jNf_gNu9WD_3AsAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fapp_2Fsettings_2Fsettings_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA8VW226jMBB9z1dYlVbaPDgyZJM0rvZLVqvKwRNwC7ZlTEi76r_vMCkXA2nTVO1TlGE8c_b4zIHF_epBr0VJ0L8ZQhk74lJwm1C0XRN9vHMxEwtJEUGssKqKaMa5kDFFoYEMBYuVgayKJyDixFIUEPKj_q8OYPapKvETbc42sSNFieAc5N3sZbY4oQgcikilylB0YOYnxvfBFp5D80zm1fG9khbn4hlop68LlqfmG0Ja1HinrFVZm9y2Cl0rLnKdsieK9im4aVkqYomFhSynKAJpwVThhyK3Yv_EIyUtSEtRrlkEeAe2hGoEhHbKcDBtQ31EuUoF98aA_Qi60MO2HMcWM00R6Sb_conNfZDFOly5O_kyU9PVh7HczQfMeRmxyxi9i9A9snC0mEOkDLNCSYqkklA9KBNhATuGqmBpmHb5hslc1KleLznvTUWTSibntdCmr76ThB2LHmOjCsnb_Wsp_P_b226k4RjRzIC0rk1h8qqPVuJVd55QcFDv5LupXF1I5R_OLMO5ZRZ_37DIigPc_J0_nc0H8_HJxLbN_tMr3noa_Jj9bMf3rvrFXBiIasYjlRaZbDbS317mqlzkByfgk4bAaMpyi6NEpHys_qvS_Do9s9h1OcJWadqsyKUjBx6_yJW_ZtGS6UULWrdxut8rk1FUaA0mYrmbOwVrwTjLOXkAWdWTeQtEBu8BftF7oDP9iaTK3SiqPa6tCr4nfYUjkYEj9Z9v5l13Oq_Ik8AM46LIByDqSiN_9W5jgo4xTSO_7WIZnVn0qu6_g_bs3Nuwg60z8SC5kBxMKvon4k_xo_RDVYaLIt7YcXbNjofX7PjA9Zovw2s_ysgi9Cl4GKfy7Y_DCTfpCHW5cVLtGfbju1x10jNf_gNu9WD_3AsAAA_22_7D = __webpack_require__(86079);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+recipes@0.5.7+6974e6bd53375d7a/node_modules/@vanilla-extract/recipes/dist/createRuntimeFn-62c9670f.esm.js
var createRuntimeFn_62c9670f_esm = __webpack_require__(8462);
;// ./src/app/settings/settings.css.ts



var externalTabLink = '_5jp6iw4';
var helpButton = '_5jp6iwe';
var helpLink = '_5jp6iwf';
var helpRow = '_5jp6iwd';
var helpSection = '_5jp6iwb';
var helpSectionTitle = '_5jp6iwc';
var pageRoot = '_5jp6iw0';
var pageTitle = '_5jp6iw1';
var section = '_5jp6iwa';
var sectionGroup = '_5jp6iw9';
var shortcutLabel = '_5jp6iwk';
var shortcutRow = '_5jp6iwj';
var shortcutsContextHeading = '_5jp6iwi';
var shortcutsContextSection = '_5jp6iwh';
var shortcutsTable = '_5jp6iwg';
var tab = (0,createRuntimeFn_62c9670f_esm.c)({defaultClassName:'_5jp6iw5',variantClassNames:{selected:{true:'_5jp6iw6',false:'_5jp6iw7'}},defaultVariants:{selected:false},compoundVariants:[]});
var tabList = '_5jp6iw3';
var tabPanel = '_5jp6iw8';
var tabRow = '_5jp6iw2';
;// ./src/app/settings/KeyboardShortcutsTab.tsx
/* __next_internal_client_entry_do_not_use__ KeyboardShortcutsTab auto */ 






const CONTEXT_LABELS = {
    global: "Global",
    "session-list": "Session List",
    approval: "Approval Review",
    terminal: "Terminal",
    cockpit: "Cockpit / Panes",
    omnibar: "Omnibar"
};
const CONTEXTS = [
    "global",
    "session-list",
    "approval",
    "terminal",
    "cockpit",
    "omnibar"
];
function formatKey(s) {
    var _s_modifiers, _s_modifiers1, _s_modifiers2, _s_modifiers3;
    if (s.sequence && s.sequence.length > 0 && s.key === undefined) return "Leader ".concat(s.sequence.join(" "));
    return [
        ((_s_modifiers = s.modifiers) === null || _s_modifiers === void 0 ? void 0 : _s_modifiers.meta) && "⌘",
        ((_s_modifiers1 = s.modifiers) === null || _s_modifiers1 === void 0 ? void 0 : _s_modifiers1.ctrl) && "Ctrl",
        ((_s_modifiers2 = s.modifiers) === null || _s_modifiers2 === void 0 ? void 0 : _s_modifiers2.shift) && "⇧",
        ((_s_modifiers3 = s.modifiers) === null || _s_modifiers3 === void 0 ? void 0 : _s_modifiers3.alt) && "⌥",
        s.key
    ].filter(Boolean).join("+");
}
/** Leader chord picker: record a chord, reset to default, or opt out ("no safe chord"). */ function LeaderKeySetting() {
    const [config, setConfig] = (0,react.useState)(leaderConfig/* getLeaderConfig */.ey);
    const [recording, setRecording] = (0,react.useState)(false);
    const [error, setError] = (0,react.useState)(null);
    (0,react.useEffect)(()=>{
        const sync = ()=>setConfig((0,leaderConfig/* getLeaderConfig */.ey)());
        window.addEventListener(leaderConfig/* LEADER_CONFIG_EVENT */.o5, sync);
        return ()=>window.removeEventListener(leaderConfig/* LEADER_CONFIG_EVENT */.o5, sync);
    }, []);
    (0,react.useEffect)(()=>{
        if (!recording) return;
        const onKeyDown = (e)=>{
            if ([
                "Alt",
                "Control",
                "Shift",
                "Meta"
            ].includes(e.key)) return; // wait for the real key
            e.preventDefault();
            e.stopPropagation();
            e.stopImmediatePropagation();
            if (e.key === "Escape") {
                setRecording(false);
                return;
            }
            const chord = (0,leaderConfig/* chordFromEvent */.FT)(e);
            if (!(0,leaderConfig/* isSafeChord */.uN)(chord)) {
                setError("Hold Alt or Meta with a key. Ctrl-only and bare keys would collide with terminal input.");
                return;
            }
            (0,leaderConfig/* setLeaderConfig */.KU)({
                enabled: true,
                chord
            });
            setError(null);
            setRecording(false);
        };
        // window capture runs before the command bar's document capture listener, so
        // recording the current leader chord does not just open the bar.
        window.addEventListener("keydown", onKeyDown, true);
        return ()=>window.removeEventListener("keydown", onKeyDown, true);
    }, [
        recording
    ]);
    const openBar = (0,react.useCallback)(()=>commandBarController/* commandBarController */.T.open([]), []);
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("section", {
        className: shortcutsContextSection,
        "data-testid": "leader-key-setting",
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                className: shortcutsContextHeading,
                children: "Command bar leader key"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: shortcutRow,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: shortcutLabel,
                        children: "Current leader"
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)(Kbd/* Kbd */.u, {
                        size: "sm",
                        children: (0,leaderConfig/* formatLeader */.RN)(config)
                    })
                ]
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsx)("p", {
                role: error ? "alert" : undefined,
                children: error !== null && error !== void 0 ? error : recording ? "Press the new chord (Esc cancels)…" : "The leader opens the command bar; the terminal never sees it."
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        onClick: ()=>{
                            setError(null);
                            setRecording((r)=>!r);
                        },
                        children: recording ? "Cancel" : "Change leader key"
                    }),
                    " ",
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        onClick: ()=>{
                            (0,leaderConfig/* resetLeaderConfig */.x7)();
                            setError(null);
                        },
                        children: "Reset to default"
                    }),
                    " ",
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        onClick: ()=>(0,leaderConfig/* setLeaderConfig */.KU)({
                                enabled: false
                            }),
                        children: "No safe chord: disable leader"
                    }),
                    " ",
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                        type: "button",
                        onClick: openBar,
                        "data-testid": "open-command-bar",
                        children: "Open command bar"
                    })
                ]
            })
        ]
    });
}
function KeyboardShortcutsTab() {
    const allShortcuts = shortcutRegistry/* registry */.u5.getAll();
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: shortcutsTable,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)(LeaderKeySetting, {}),
            CONTEXTS.map((ctx)=>{
                const shortcuts = allShortcuts[ctx];
                if (!shortcuts || shortcuts.length === 0) return null;
                return /*#__PURE__*/ (0,jsx_runtime.jsxs)("section", {
                    className: shortcutsContextSection,
                    children: [
                        /*#__PURE__*/ (0,jsx_runtime.jsx)("h3", {
                            className: shortcutsContextHeading,
                            children: CONTEXT_LABELS[ctx]
                        }),
                        shortcuts.map((s)=>{
                            var _s_sequence;
                            var _s_sequence_join;
                            return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                className: shortcutRow,
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                                        className: shortcutLabel,
                                        children: s.label
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("span", {
                                        children: [
                                            s.sequence && s.key !== undefined && /*#__PURE__*/ (0,jsx_runtime.jsx)(Kbd/* Kbd */.u, {
                                                size: "sm",
                                                children: "Leader ".concat(s.sequence.join(" "))
                                            }),
                                            " ",
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)(Kbd/* Kbd */.u, {
                                                size: "sm",
                                                children: formatKey(s)
                                            })
                                        ]
                                    })
                                ]
                            }, "".concat(s.label, "-").concat((_s_sequence_join = (_s_sequence = s.sequence) === null || _s_sequence === void 0 ? void 0 : _s_sequence.join("")) !== null && _s_sequence_join !== void 0 ? _s_sequence_join : s.key));
                        })
                    ]
                }, ctx);
            })
        ]
    });
}

// EXTERNAL MODULE: ./src/lib/analytics/usePageView.ts
var usePageView = __webpack_require__(90814);
// EXTERNAL MODULE: ./src/lib/contexts/OnboardingContext.tsx + 3 modules
var OnboardingContext = __webpack_require__(7038);
// EXTERNAL MODULE: ./src/lib/contexts/FeatureFlagsContext.tsx
var FeatureFlagsContext = __webpack_require__(63367);
// EXTERNAL MODULE: ./src/lib/routes.ts + 1 modules
var routes = __webpack_require__(76514);
;// ./src/app/settings/page.tsx
/* __next_internal_client_entry_do_not_use__ default auto */ 
// analytics-exempt




















function SettingsPageInner() {
    (0,usePageView/* usePageView */.O)();
    const { triggerOnboarding } = (0,OnboardingContext/* useOnboardingContext */.w)();
    const { flags } = (0,FeatureFlagsContext/* useFeatureFlags */.h)();
    const searchParams = (0,nav/* useSearchParams */.ok)();
    const tabParam = searchParams.get("tab");
    const validTabs = [
        "general",
        "config-files",
        "appearance",
        "keyboard-shortcuts"
    ];
    const defaultValue = validTabs.includes(tabParam !== null && tabParam !== void 0 ? tabParam : "") ? tabParam : "general";
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("main", {
        id: "main-content",
        className: pageRoot,
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("h1", {
                className: pageTitle,
                children: "Settings"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)(dist/* Root */.bL, {
                defaultValue: defaultValue,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                        className: tabRow,
                        children: [
                            /*#__PURE__*/ (0,jsx_runtime.jsxs)(dist/* List */.B8, {
                                className: tabList,
                                "aria-label": "Settings tabs",
                                children: [
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)(dist/* Trigger */.l9, {
                                        value: "general",
                                        className: tab({}),
                                        children: "General"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)(dist/* Trigger */.l9, {
                                        value: "config-files",
                                        className: tab({}),
                                        children: "Config Files"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)(dist/* Trigger */.l9, {
                                        value: "appearance",
                                        className: tab({}),
                                        children: "Appearance"
                                    }),
                                    /*#__PURE__*/ (0,jsx_runtime.jsx)(dist/* Trigger */.l9, {
                                        value: "keyboard-shortcuts",
                                        className: tab({}),
                                        children: "Keyboard Shortcuts"
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,jsx_runtime.jsx)(nav/* Link */.N_, {
                                href: routes/* routes */.J.settingsPipelineModes,
                                className: externalTabLink,
                                "data-testid": "settings-pipeline-modes-tab-link",
                                children: "Pipeline Modes ↗"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)(dist/* Content */.UC, {
                        value: "general",
                        className: tabPanel,
                        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: sectionGroup,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("section", {
                                    className: section,
                                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)(GlobalDefaultsForm, {})
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("section", {
                                    className: section,
                                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)(ProfilesManager, {})
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("section", {
                                    className: section,
                                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)(DirectoryRulesManager, {})
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("section", {
                                    className: section,
                                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)(AliasesManager, {})
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("section", {
                                    className: section,
                                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)(ProgramsManager, {})
                                }),
                                flags["backlog"] && /*#__PURE__*/ (0,jsx_runtime.jsx)("section", {
                                    className: section,
                                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)(nav/* Link */.N_, {
                                        href: routes/* routes */.J.settingsBacklogSources,
                                        className: helpLink,
                                        children: "Backlog Sources (GitHub sync) →"
                                    })
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("section", {
                                    className: section,
                                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)(nav/* Link */.N_, {
                                        href: routes/* routes */.J.settingsPipelineModes,
                                        className: helpLink,
                                        children: "Pipeline Modes →"
                                    })
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("section", {
                                    className: section,
                                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)(nav/* Link */.N_, {
                                        href: routes/* routes */.J.settingsBacklogStages,
                                        className: helpLink,
                                        "data-testid": "settings-backlog-stages-link",
                                        children: "Backlog Stages →"
                                    })
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("section", {
                                    className: section,
                                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)(nav/* Link */.N_, {
                                        href: routes/* routes */.J.settingsRemotes,
                                        className: helpLink,
                                        "data-testid": "settings-remotes-link",
                                        children: "Remotes (SSH hosts) →"
                                    })
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("section", {
                                    className: section,
                                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)(nav/* Link */.N_, {
                                        href: routes/* routes */.J.settingsJules,
                                        className: helpLink,
                                        "data-testid": "settings-jules-link",
                                        children: "Jules (Google cloud agent) →"
                                    })
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("section", {
                                    className: section,
                                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)(nav/* Link */.N_, {
                                        href: routes/* routes */.J.settingsTaggingClassifier,
                                        className: helpLink,
                                        "data-testid": "settings-tagging-classifier-link",
                                        children: "Tag Classification (AI tagging model) →"
                                    })
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("section", {
                                    className: section,
                                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)(nav/* Link */.N_, {
                                        href: routes/* routes */.J.settingsLlmBackends,
                                        className: helpLink,
                                        "data-testid": "settings-llm-backends-link",
                                        children: "LLM Backends (claude / consolette / agy / gemini / opencode) →"
                                    })
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("section", {
                                    className: section,
                                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)(nav/* Link */.N_, {
                                        href: routes/* routes */.J.settingsBackgroundModels,
                                        className: helpLink,
                                        "data-testid": "settings-background-models-link",
                                        children: "Background Models (cheaper models and effort for unattended work) →"
                                    })
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("section", {
                                    className: section,
                                    children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                        className: helpSection,
                                        children: [
                                            /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                                className: helpSectionTitle,
                                                children: "Help"
                                            }),
                                            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                                                className: helpRow,
                                                children: [
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                                        className: helpButton,
                                                        onClick: triggerOnboarding,
                                                        children: "Show onboarding tour again"
                                                    }),
                                                    /*#__PURE__*/ (0,jsx_runtime.jsx)(nav/* Link */.N_, {
                                                        href: routes/* routes */.J.help,
                                                        className: helpLink,
                                                        children: "View documentation"
                                                    }),
                                                    flags["changelog"] && /*#__PURE__*/ (0,jsx_runtime.jsx)(nav/* Link */.N_, {
                                                        href: routes/* routes */.J.changelog,
                                                        className: helpLink,
                                                        "data-testid": "settings-changelog-link",
                                                        children: "What's new (changelog)"
                                                    })
                                                ]
                                            })
                                        ]
                                    })
                                })
                            ]
                        })
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)(dist/* Content */.UC, {
                        value: "config-files",
                        className: tabPanel,
                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)(react.Suspense, {
                            fallback: /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                children: "Loading..."
                            }),
                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)(ConfigPageContent, {})
                        })
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)(dist/* Content */.UC, {
                        value: "appearance",
                        className: tabPanel,
                        children: /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                            className: sectionGroup,
                            children: [
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("section", {
                                    className: section,
                                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)(ThemePicker, {})
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("section", {
                                    className: section,
                                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)(InputModeSetting, {})
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("section", {
                                    className: section,
                                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)(TerminalSettings, {})
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("section", {
                                    className: section,
                                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)(PushNotificationSettings, {})
                                }),
                                /*#__PURE__*/ (0,jsx_runtime.jsx)("section", {
                                    className: section,
                                    children: /*#__PURE__*/ (0,jsx_runtime.jsx)(SlackNotificationSettings, {})
                                })
                            ]
                        })
                    }),
                    /*#__PURE__*/ (0,jsx_runtime.jsx)(dist/* Content */.UC, {
                        value: "keyboard-shortcuts",
                        className: tabPanel,
                        children: /*#__PURE__*/ (0,jsx_runtime.jsx)(react.Suspense, {
                            fallback: /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                                children: "Loading..."
                            }),
                            children: /*#__PURE__*/ (0,jsx_runtime.jsx)(KeyboardShortcutsTab, {})
                        })
                    })
                ]
            })
        ]
    });
}
function SettingsPage() {
    return /*#__PURE__*/ (0,jsx_runtime.jsx)(react.Suspense, {
        fallback: /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
            children: "Loading settings..."
        }),
        children: /*#__PURE__*/ (0,jsx_runtime.jsx)(SettingsPageInner, {})
    });
}


/***/ }),

/***/ 96307:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  w: () => (/* binding */ InlineNotice)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/compiled/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(61574);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(65916);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fcommon%2FInlineNotice.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA62U3W6DMAyF7_sUuVwvMhXarhp7mCklBryBw0zo39R3nwKlggDVJu0GQY45sT87eX4PktNmu9uuxPdCCI1VmatzJJIcTm8LIVSOKUm0UFTtoqysYuukVJWROCh-kvI9eIVLyJfwsnRKqbRGSn11vWrUvWENHImgPInK5Ki9sKIXJVlprKuR066NUfFnyqYm7QcEbUBscsO-1maRGLKywguMvPfLt8V18dyBCRowk07Ba2vVYMkY6TMSK7eSI4HMANPMjuz10D6c5t54amSILRqK3P51QY-xu38iEbjXAkkeUdusSai33Xq-mtUwsc10Yo-3l0d2unsOzLaNWb9fZAj609B9xzVXLrfSIFng2S527B-28aYfpzuRDoe16ZyFk5UaYsOqBV-TBnb9HNYTZeYAPA-ThjBfRvXPz7zPZu4MvCz_wuvrv3F5M3BeTp32PoIesnkQHrfdv89N-CsO9yrDspn6QnGKJHNIbCRUbc3olPvHLtzc_kS6x3Rrj2_ZGLoCPurKYnKWsSELZPuSf-dcfwAzY517yQUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fcommon_2FInlineNotice_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA62U3W6DMAyF7_sUuVwvMhXarhp7mCklBryBw0zo39R3nwKlggDVJu0GQY45sT87eX4PktNmu9uuxPdCCI1VmatzJJIcTm8LIVSOKUm0UFTtoqysYuukVJWROCh_kvI9eIVLyJfwsnRKqbRGSn11vWrUvWENHImgPInK5Ki9sKIXJVlprKuR066NUfFnyqYm7QcEbUBscsO_1maRGLKywguMvPfLt8V18dyBCRowk07Ba2vVYMkY6TMSK7eSI4HMANPMjuz10D6c5t54amSILRqK3P51QY_xu38iEbjXAkkeUdusSai33Xq_mtUwsc10Yo_3l0d2unsOzLaNWb9fZAj609B9xzVXLrfSIFng2S527B_28aYfpzuRDoe16ZyFk5UaYsOqBV_TBnb9HNYTZeYAPA_ThjBfRvXPz7zPZu4MvCz_wuvrv3F5M3BeTp32PoIesnkQHrfdv89N_CsO9yrDspn6QnGKJHNIbCRUbc3olPvHLtzc_kS6x3Rrj2_ZGLoCPurKYnKWsSELZPuSf_dcfwAzY517yQUAAA_22_7D = __webpack_require__(29068);
;// ./src/components/common/InlineNotice.css.ts


var actionButton = '_1fx45755';
var actionButtonPrimary = '_1fx45756';
var actions = '_1fx45754';
var body = '_1fx45752';
var container = '_1fx45750';
var dismissButton = '_1fx45757';
var icon = '_1fx45751';
var messageText = '_1fx45753';
;// ./src/components/common/InlineNotice.tsx
/* __next_internal_client_entry_do_not_use__ InlineNotice auto */ 
// +feature: backlog:inline-notice
// InlineNotice — informational (non-error) banner family, added for
// project_plans/backlog-event-driven-updates Epic 5.3 (Task 5.3.2b) and
// reused for Task 5.3.1c's terminal-state banner. Deliberately NOT built on
// InlineError: InlineError is role="alert" aria-live="assertive" (correct
// for real failures), which is the wrong politeness level for a routine,
// non-blocking notice like "this item changed elsewhere" or "this item was
// archived elsewhere" (design/ux.md §6). This component is
// role="status" aria-live="polite" instead, and never blocks interaction
// with the rest of the surrounding form/panel.

function InlineNotice(param) {
    let { message, actions: noticeActions, onDismiss, ...rest } = param;
    return /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
        className: container,
        role: "status",
        "aria-live": "polite",
        "data-testid": rest["data-testid"],
        children: [
            /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                className: icon,
                "aria-hidden": "true",
                children: "ⓘ"
            }),
            /*#__PURE__*/ (0,jsx_runtime.jsxs)("div", {
                className: body,
                children: [
                    /*#__PURE__*/ (0,jsx_runtime.jsx)("span", {
                        className: messageText,
                        children: message
                    }),
                    noticeActions && noticeActions.length > 0 && /*#__PURE__*/ (0,jsx_runtime.jsx)("div", {
                        className: actions,
                        children: noticeActions.map((a)=>/*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                                type: "button",
                                className: a.variant === "primary" ? actionButtonPrimary : actionButton,
                                onClick: a.onClick,
                                children: a.label
                            }, a.label))
                    })
                ]
            }),
            onDismiss && /*#__PURE__*/ (0,jsx_runtime.jsx)("button", {
                type: "button",
                className: dismissButton,
                onClick: onDismiss,
                "aria-label": "Dismiss notice",
                children: "\xd7"
            })
        ]
    });
}


/***/ })

},
/******/ __webpack_require__ => { // webpackRuntimeModules
/******/ var __webpack_exec__ = (moduleId) => (__webpack_require__(__webpack_require__.s = moduleId))
/******/ __webpack_require__.O(0, [1869,852,4202,9353,8182,7924,6000,4069,820,2477,8165,3575,8890,2513,2275,3117,4247,218,7461,9691,3516,7358], () => (__webpack_exec__(31813)));
/******/ var __webpack_exports__ = __webpack_require__.O();
/******/ _N_E = __webpack_exports__;
/******/ }
]);
//# sourceMappingURL=page-dd44c53c7d2bf4b5.js.map