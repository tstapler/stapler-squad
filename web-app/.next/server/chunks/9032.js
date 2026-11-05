exports.id = 9032;
exports.ids = [9032];
exports.modules = {

/***/ 15512:
/***/ ((module, exports, __webpack_require__) => {

/* eslint-env browser */

/**
 * This is the web browser implementation of `debug()`.
 */

exports.formatArgs = formatArgs;
exports.save = save;
exports.load = load;
exports.useColors = useColors;
exports.storage = localstorage();
exports.destroy = (() => {
	let warned = false;

	return () => {
		if (!warned) {
			warned = true;
			console.warn('Instance method `debug.destroy()` is deprecated and no longer does anything. It will be removed in the next major version of `debug`.');
		}
	};
})();

/**
 * Colors.
 */

exports.colors = [
	'#0000CC',
	'#0000FF',
	'#0033CC',
	'#0033FF',
	'#0066CC',
	'#0066FF',
	'#0099CC',
	'#0099FF',
	'#00CC00',
	'#00CC33',
	'#00CC66',
	'#00CC99',
	'#00CCCC',
	'#00CCFF',
	'#3300CC',
	'#3300FF',
	'#3333CC',
	'#3333FF',
	'#3366CC',
	'#3366FF',
	'#3399CC',
	'#3399FF',
	'#33CC00',
	'#33CC33',
	'#33CC66',
	'#33CC99',
	'#33CCCC',
	'#33CCFF',
	'#6600CC',
	'#6600FF',
	'#6633CC',
	'#6633FF',
	'#66CC00',
	'#66CC33',
	'#9900CC',
	'#9900FF',
	'#9933CC',
	'#9933FF',
	'#99CC00',
	'#99CC33',
	'#CC0000',
	'#CC0033',
	'#CC0066',
	'#CC0099',
	'#CC00CC',
	'#CC00FF',
	'#CC3300',
	'#CC3333',
	'#CC3366',
	'#CC3399',
	'#CC33CC',
	'#CC33FF',
	'#CC6600',
	'#CC6633',
	'#CC9900',
	'#CC9933',
	'#CCCC00',
	'#CCCC33',
	'#FF0000',
	'#FF0033',
	'#FF0066',
	'#FF0099',
	'#FF00CC',
	'#FF00FF',
	'#FF3300',
	'#FF3333',
	'#FF3366',
	'#FF3399',
	'#FF33CC',
	'#FF33FF',
	'#FF6600',
	'#FF6633',
	'#FF9900',
	'#FF9933',
	'#FFCC00',
	'#FFCC33'
];

/**
 * Currently only WebKit-based Web Inspectors, Firefox >= v31,
 * and the Firebug extension (any Firefox version) are known
 * to support "%c" CSS customizations.
 *
 * TODO: add a `localStorage` variable to explicitly enable/disable colors
 */

// eslint-disable-next-line complexity
function useColors() {
	// NB: In an Electron preload script, document will be defined but not fully
	// initialized. Since we know we're in Chrome, we'll just detect this case
	// explicitly
	if (typeof window !== 'undefined' && window.process && (window.process.type === 'renderer' || window.process.__nwjs)) {
		return true;
	}

	// Internet Explorer and Edge do not support colors.
	if (typeof navigator !== 'undefined' && navigator.userAgent && navigator.userAgent.toLowerCase().match(/(edge|trident)\/(\d+)/)) {
		return false;
	}

	let m;

	// Is webkit? http://stackoverflow.com/a/16459606/376773
	// document is undefined in react-native: https://github.com/facebook/react-native/pull/1632
	// eslint-disable-next-line no-return-assign
	return (typeof document !== 'undefined' && document.documentElement && document.documentElement.style && document.documentElement.style.WebkitAppearance) ||
		// Is firebug? http://stackoverflow.com/a/398120/376773
		(typeof window !== 'undefined' && window.console && (window.console.firebug || (window.console.exception && window.console.table))) ||
		// Is firefox >= v31?
		// https://developer.mozilla.org/en-US/docs/Tools/Web_Console#Styling_messages
		(typeof navigator !== 'undefined' && navigator.userAgent && (m = navigator.userAgent.toLowerCase().match(/firefox\/(\d+)/)) && parseInt(m[1], 10) >= 31) ||
		// Double check webkit in userAgent just in case we are in a worker
		(typeof navigator !== 'undefined' && navigator.userAgent && navigator.userAgent.toLowerCase().match(/applewebkit\/(\d+)/));
}

/**
 * Colorize log arguments if enabled.
 *
 * @api public
 */

function formatArgs(args) {
	args[0] = (this.useColors ? '%c' : '') +
		this.namespace +
		(this.useColors ? ' %c' : ' ') +
		args[0] +
		(this.useColors ? '%c ' : ' ') +
		'+' + module.exports.humanize(this.diff);

	if (!this.useColors) {
		return;
	}

	const c = 'color: ' + this.color;
	args.splice(1, 0, c, 'color: inherit');

	// The final "%c" is somewhat tricky, because there could be other
	// arguments passed either before or after the %c, so we need to
	// figure out the correct index to insert the CSS into
	let index = 0;
	let lastC = 0;
	args[0].replace(/%[a-zA-Z%]/g, match => {
		if (match === '%%') {
			return;
		}
		index++;
		if (match === '%c') {
			// We only are interested in the *last* %c
			// (the user may have provided their own)
			lastC = index;
		}
	});

	args.splice(lastC, 0, c);
}

/**
 * Invokes `console.debug()` when available.
 * No-op when `console.debug` is not a "function".
 * If `console.debug` is not available, falls back
 * to `console.log`.
 *
 * @api public
 */
exports.log = console.debug || console.log || (() => {});

/**
 * Save `namespaces`.
 *
 * @param {String} namespaces
 * @api private
 */
function save(namespaces) {
	try {
		if (namespaces) {
			exports.storage.setItem('debug', namespaces);
		} else {
			exports.storage.removeItem('debug');
		}
	} catch (error) {
		// Swallow
		// XXX (@Qix-) should we be logging these?
	}
}

/**
 * Load `namespaces`.
 *
 * @return {String} returns the previously persisted debug modes
 * @api private
 */
function load() {
	let r;
	try {
		r = exports.storage.getItem('debug') || exports.storage.getItem('DEBUG') ;
	} catch (error) {
		// Swallow
		// XXX (@Qix-) should we be logging these?
	}

	// If debug isn't set in LS, and we're in Electron, try to load $DEBUG
	if (!r && typeof process !== 'undefined' && 'env' in process) {
		r = process.env.DEBUG;
	}

	return r;
}

/**
 * Localstorage attempts to return the localstorage.
 *
 * This is necessary because safari throws
 * when a user disables cookies/localstorage
 * and you attempt to access it.
 *
 * @return {LocalStorage}
 * @api private
 */

function localstorage() {
	try {
		// TVMLKit (Apple TV JS Runtime) does not have a window object, just localStorage in the global context
		// The Browser also has localStorage in the global context.
		return localStorage;
	} catch (error) {
		// Swallow
		// XXX (@Qix-) should we be logging these?
	}
}

module.exports = __webpack_require__(43155)(exports);

const {formatters} = module.exports;

/**
 * Map %j to `JSON.stringify()`, since no Web Inspectors do that by default.
 */

formatters.j = function (v) {
	try {
		return JSON.stringify(v);
	} catch (error) {
		return '[UnexpectedJSONParseError]: ' + error.message;
	}
};


/***/ }),

/***/ 21539:
/***/ ((__unused_webpack_module, exports) => {

// Unless explicitly stated otherwise all files in this repository are licensed under the Apache 2.0 License.
//
// This product includes software developed at Datadog (https://www.datadoghq.com/). Copyright 2021 Datadog, Inc.

const importHooks = [] // TODO should this be a Set?
const setters = new WeakMap()
const getters = new WeakMap()
const specifiers = new Map()
const toHook = []

const proxyHandler = {
  set (target, name, value) {
    const set = setters.get(target)
    const setter = set && set[name]
    if (typeof setter === 'function') {
      return setter(value)
    }
    // If a module doesn't export the property being assigned (e.g. no default
    // export), there is no setter to call. Don't crash userland code.
    return true
  },

  get (target, name) {
    if (name === Symbol.toStringTag) {
      return 'Module'
    }

    const getter = getters.get(target)[name]

    if (typeof getter === 'function') {
      return getter()
    }
  },

  defineProperty (target, property, descriptor) {
    if ((!('value' in descriptor))) {
      throw new Error('Getters/setters are not supported for exports property descriptors.')
    }

    const set = setters.get(target)
    const setter = set && set[property]
    if (typeof setter === 'function') {
      return setter(descriptor.value)
    }
    return true
  }
}

function register (name, namespace, set, get, specifier) {
  specifiers.set(name, specifier)
  setters.set(namespace, set)
  getters.set(namespace, get)
  const proxy = new Proxy(namespace, proxyHandler)
  importHooks.forEach(hook => hook(name, proxy, specifier))
  toHook.push([name, proxy, specifier])
}

// Delays (ms) for re-reading exports that were still in their temporal dead zone
// when the wrapper first ran (circular imports). Retried on a microtask first,
// then at these intervals; unref'd so best-effort retries never hold the process
// open. Frozen once at module load rather than rebuilt per wrapper.
const RETRY_DELAYS = [0, 10, 50]

/**
 * Per-wrapped-module state a generated wrapper builds once to expose its exports
 * through iitm's proxy. Each wrapper holds a local binding per export plus
 * `write`/`read` closures over it; `bind` seeds that binding from the real
 * module and installs the proxy's `set`/`get` for the name, and `flush` resolves
 * any export that was undefined (circular import) once it becomes available.
 *
 * This is the boilerplate the wrapper used to inline in full per module. Hoisting
 * it here compiles it once instead of once per wrapped module and keeps the
 * per-export bind call site monomorphic.
 */
class ModuleBinder {
  // Mimics a Module namespace object (https://tc39.es/ecma262/#sec-module-namespace-objects).
  namespace = Object.create(null, { [Symbol.toStringTag]: { value: 'Module' } })
  set = {}
  get = {}
  #overridden = Object.create(null)
  #pending = []

  /**
   * Seeds `key` from `source` and installs its proxy accessors. A value that is
   * undefined or throws `ReferenceError` (temporal dead zone during a circular
   * import) is deferred to `flush`; any other throw propagates.
   *
   * @param {string} key The export name.
   * @param {object} source The real module namespace to read the value from.
   * @param {(value: unknown) => void} write Assigns the wrapper's local binding.
   * @param {() => unknown} read Reads the wrapper's local binding.
   * @param {boolean} useFallback Fall back to `source.default` (the synthetic
   * `module.exports` name a builtin does not expose on its ESM namespace).
   * @returns {void}
   */
  bind (key, source, write, read, useFallback) {
    const readSource = useFallback
      ? () => source[key] ?? source.default
      : () => source[key]
    this.#overridden[key] = false
    let deferred = false
    try {
      const value = readSource()
      write(value)
      this.namespace[key] = value
    } catch (error) {
      if (!(error instanceof ReferenceError)) throw error
      deferred = true
    }
    if (deferred || read() === undefined) {
      this.#pending.push(this.#makeUpdater(key, readSource, write))
    }
    this.set[key] = (value) => {
      this.#overridden[key] = true
      write(value)
      return true
    }
    this.get[key] = read
  }

  /**
   * @param {string} key The export name to update.
   * @param {() => unknown} readSource Reads the current value from the real module.
   * @param {(value: unknown) => void} write Assigns the wrapper's local binding.
   * @returns {() => boolean} Updater returning whether the value is now settled.
   */
  #makeUpdater (key, readSource, write) {
    return () => {
      if (this.#overridden[key] === true) return true
      try {
        const value = readSource()
        if (value !== undefined) {
          write(value)
          this.namespace[key] = value
          return true
        }
        return false
      } catch (error) {
        if (error instanceof ReferenceError) return false
        // Only reached if a getter starts throwing a non-ReferenceError after the
        // initial bind read already succeeded or deferred; surfaces in flush's
        // microtask. Kept as-is from the inline wrapper.
        /* c8 ignore next */
        throw error
      }
    }
  }

  #flushOnce () {
    const next = []
    for (const updater of this.#pending) {
      // If it still throws ReferenceError, keep it for the (single) next attempt.
      if (updater() !== true) next.push(updater)
    }
    this.#pending = next
  }

  /**
   * Resolves exports deferred by `bind` (undefined or TDZ at wrapper-eval time).
   * Retries on a microtask, then at `RETRY_DELAYS`, giving up afterwards to avoid
   * unbounded retries. A no-op when nothing was deferred.
   *
   * @returns {void}
   */
  flush () {
    if (this.#pending.length === 0) return
    queueMicrotask(() => {
      this.#flushOnce()
      this.#scheduleRetry(0)
    })
  }

  /**
   * @param {number} attempt Index into `RETRY_DELAYS` for the next retry.
   * @returns {void}
   */
  #scheduleRetry (attempt) {
    if (this.#pending.length === 0) return
    if (attempt >= RETRY_DELAYS.length) {
      // Give up: leave exports as-is to avoid unbounded retries.
      this.#pending = []
      return
    }
    const timer = setTimeout(() => {
      this.#flushOnce()
      this.#scheduleRetry(attempt + 1)
    }, RETRY_DELAYS[attempt])
    // Don't keep the process alive just for best-effort retries.
    if (timer && typeof timer.unref === 'function') timer.unref()
  }
}

exports.register = register
exports.ModuleBinder = ModuleBinder
exports.importHooks = importHooks
exports.specifiers = specifiers
exports.toHook = toHook


/***/ }),

/***/ 30038:
/***/ ((module, exports, __webpack_require__) => {

/**
 * Module dependencies.
 */

const tty = __webpack_require__(83997);
const util = __webpack_require__(28354);

/**
 * This is the Node.js implementation of `debug()`.
 */

exports.init = init;
exports.log = log;
exports.formatArgs = formatArgs;
exports.save = save;
exports.load = load;
exports.useColors = useColors;
exports.destroy = util.deprecate(
	() => {},
	'Instance method `debug.destroy()` is deprecated and no longer does anything. It will be removed in the next major version of `debug`.'
);

/**
 * Colors.
 */

exports.colors = [6, 2, 3, 4, 5, 1];

try {
	// Optional dependency (as in, doesn't need to be installed, NOT like optionalDependencies in package.json)
	// eslint-disable-next-line import/no-extraneous-dependencies
	const supportsColor = __webpack_require__(90990);

	if (supportsColor && (supportsColor.stderr || supportsColor).level >= 2) {
		exports.colors = [
			20,
			21,
			26,
			27,
			32,
			33,
			38,
			39,
			40,
			41,
			42,
			43,
			44,
			45,
			56,
			57,
			62,
			63,
			68,
			69,
			74,
			75,
			76,
			77,
			78,
			79,
			80,
			81,
			92,
			93,
			98,
			99,
			112,
			113,
			128,
			129,
			134,
			135,
			148,
			149,
			160,
			161,
			162,
			163,
			164,
			165,
			166,
			167,
			168,
			169,
			170,
			171,
			172,
			173,
			178,
			179,
			184,
			185,
			196,
			197,
			198,
			199,
			200,
			201,
			202,
			203,
			204,
			205,
			206,
			207,
			208,
			209,
			214,
			215,
			220,
			221
		];
	}
} catch (error) {
	// Swallow - we only care if `supports-color` is available; it doesn't have to be.
}

/**
 * Build up the default `inspectOpts` object from the environment variables.
 *
 *   $ DEBUG_COLORS=no DEBUG_DEPTH=10 DEBUG_SHOW_HIDDEN=enabled node script.js
 */

exports.inspectOpts = Object.keys(process.env).filter(key => {
	return /^debug_/i.test(key);
}).reduce((obj, key) => {
	// Camel-case
	const prop = key
		.substring(6)
		.toLowerCase()
		.replace(/_([a-z])/g, (_, k) => {
			return k.toUpperCase();
		});

	// Coerce string value into JS value
	let val = process.env[key];
	if (/^(yes|on|true|enabled)$/i.test(val)) {
		val = true;
	} else if (/^(no|off|false|disabled)$/i.test(val)) {
		val = false;
	} else if (val === 'null') {
		val = null;
	} else {
		val = Number(val);
	}

	obj[prop] = val;
	return obj;
}, {});

/**
 * Is stdout a TTY? Colored output is enabled when `true`.
 */

function useColors() {
	return 'colors' in exports.inspectOpts ?
		Boolean(exports.inspectOpts.colors) :
		tty.isatty(process.stderr.fd);
}

/**
 * Adds ANSI color escape codes if enabled.
 *
 * @api public
 */

function formatArgs(args) {
	const {namespace: name, useColors} = this;

	if (useColors) {
		const c = this.color;
		const colorCode = '\u001B[3' + (c < 8 ? c : '8;5;' + c);
		const prefix = `  ${colorCode};1m${name} \u001B[0m`;

		args[0] = prefix + args[0].split('\n').join('\n' + prefix);
		args.push(colorCode + 'm+' + module.exports.humanize(this.diff) + '\u001B[0m');
	} else {
		args[0] = getDate() + name + ' ' + args[0];
	}
}

function getDate() {
	if (exports.inspectOpts.hideDate) {
		return '';
	}
	return new Date().toISOString() + ' ';
}

/**
 * Invokes `util.formatWithOptions()` with the specified arguments and writes to stderr.
 */

function log(...args) {
	return process.stderr.write(util.formatWithOptions(exports.inspectOpts, ...args) + '\n');
}

/**
 * Save `namespaces`.
 *
 * @param {String} namespaces
 * @api private
 */
function save(namespaces) {
	if (namespaces) {
		process.env.DEBUG = namespaces;
	} else {
		// If you set a process.env field to null or undefined, it gets cast to the
		// string 'null' or 'undefined'. Just delete instead.
		delete process.env.DEBUG;
	}
}

/**
 * Load `namespaces`.
 *
 * @return {String} returns the previously persisted debug modes
 * @api private
 */

function load() {
	return process.env.DEBUG;
}

/**
 * Init logic for `debug` instances.
 *
 * Create a new `inspectOpts` object in case `useColors` is set
 * differently for a particular `debug` instance.
 */

function init(debug) {
	debug.inspectOpts = {};

	const keys = Object.keys(exports.inspectOpts);
	for (let i = 0; i < keys.length; i++) {
		debug.inspectOpts[keys[i]] = exports.inspectOpts[keys[i]];
	}
}

module.exports = __webpack_require__(43155)(exports);

const {formatters} = module.exports;

/**
 * Map %o to `util.inspect()`, all on a single line.
 */

formatters.o = function (v) {
	this.inspectOpts.colors = this.useColors;
	return util.inspect(v, this.inspectOpts)
		.split('\n')
		.map(str => str.trim())
		.join(' ');
};

/**
 * Map %O to `util.inspect()`, allowing multiple lines if needed.
 */

formatters.O = function (v) {
	this.inspectOpts.colors = this.useColors;
	return util.inspect(v, this.inspectOpts);
};


/***/ }),

/***/ 40900:
/***/ ((module, __unused_webpack_exports, __webpack_require__) => {

/**
 * Detect Electron renderer / nwjs process, which is node, but we should
 * treat as a browser.
 */

if (typeof process === 'undefined' || process.type === 'renderer' || false === true || process.__nwjs) {
	module.exports = __webpack_require__(15512);
} else {
	module.exports = __webpack_require__(30038);
}


/***/ }),

/***/ 43155:
/***/ ((module, __unused_webpack_exports, __webpack_require__) => {


/**
 * This is the common logic for both the Node.js and web browser
 * implementations of `debug()`.
 */

function setup(env) {
	createDebug.debug = createDebug;
	createDebug.default = createDebug;
	createDebug.coerce = coerce;
	createDebug.disable = disable;
	createDebug.enable = enable;
	createDebug.enabled = enabled;
	createDebug.humanize = __webpack_require__(58221);
	createDebug.destroy = destroy;

	Object.keys(env).forEach(key => {
		createDebug[key] = env[key];
	});

	/**
	* The currently active debug mode names, and names to skip.
	*/

	createDebug.names = [];
	createDebug.skips = [];

	/**
	* Map of special "%n" handling functions, for the debug "format" argument.
	*
	* Valid key names are a single, lower or upper-case letter, i.e. "n" and "N".
	*/
	createDebug.formatters = {};

	/**
	* Selects a color for a debug namespace
	* @param {String} namespace The namespace string for the debug instance to be colored
	* @return {Number|String} An ANSI color code for the given namespace
	* @api private
	*/
	function selectColor(namespace) {
		let hash = 0;

		for (let i = 0; i < namespace.length; i++) {
			hash = ((hash << 5) - hash) + namespace.charCodeAt(i);
			hash |= 0; // Convert to 32bit integer
		}

		return createDebug.colors[Math.abs(hash) % createDebug.colors.length];
	}
	createDebug.selectColor = selectColor;

	/**
	* Create a debugger with the given `namespace`.
	*
	* @param {String} namespace
	* @return {Function}
	* @api public
	*/
	function createDebug(namespace) {
		let prevTime;
		let enableOverride = null;
		let namespacesCache;
		let enabledCache;

		function debug(...args) {
			// Disabled?
			if (!debug.enabled) {
				return;
			}

			const self = debug;

			// Set `diff` timestamp
			const curr = Number(new Date());
			const ms = curr - (prevTime || curr);
			self.diff = ms;
			self.prev = prevTime;
			self.curr = curr;
			prevTime = curr;

			args[0] = createDebug.coerce(args[0]);

			if (typeof args[0] !== 'string') {
				// Anything else let's inspect with %O
				args.unshift('%O');
			}

			// Apply any `formatters` transformations
			let index = 0;
			args[0] = args[0].replace(/%([a-zA-Z%])/g, (match, format) => {
				// If we encounter an escaped % then don't increase the array index
				if (match === '%%') {
					return '%';
				}
				index++;
				const formatter = createDebug.formatters[format];
				if (typeof formatter === 'function') {
					const val = args[index];
					match = formatter.call(self, val);

					// Now we need to remove `args[index]` since it's inlined in the `format`
					args.splice(index, 1);
					index--;
				}
				return match;
			});

			// Apply env-specific formatting (colors, etc.)
			createDebug.formatArgs.call(self, args);

			const logFn = self.log || createDebug.log;
			logFn.apply(self, args);
		}

		debug.namespace = namespace;
		debug.useColors = createDebug.useColors();
		debug.color = createDebug.selectColor(namespace);
		debug.extend = extend;
		debug.destroy = createDebug.destroy; // XXX Temporary. Will be removed in the next major release.

		Object.defineProperty(debug, 'enabled', {
			enumerable: true,
			configurable: false,
			get: () => {
				if (enableOverride !== null) {
					return enableOverride;
				}
				if (namespacesCache !== createDebug.namespaces) {
					namespacesCache = createDebug.namespaces;
					enabledCache = createDebug.enabled(namespace);
				}

				return enabledCache;
			},
			set: v => {
				enableOverride = v;
			}
		});

		// Env-specific initialization logic for debug instances
		if (typeof createDebug.init === 'function') {
			createDebug.init(debug);
		}

		return debug;
	}

	function extend(namespace, delimiter) {
		const newDebug = createDebug(this.namespace + (typeof delimiter === 'undefined' ? ':' : delimiter) + namespace);
		newDebug.log = this.log;
		return newDebug;
	}

	/**
	* Enables a debug mode by namespaces. This can include modes
	* separated by a colon and wildcards.
	*
	* @param {String} namespaces
	* @api public
	*/
	function enable(namespaces) {
		createDebug.save(namespaces);
		createDebug.namespaces = namespaces;

		createDebug.names = [];
		createDebug.skips = [];

		const split = (typeof namespaces === 'string' ? namespaces : '')
			.trim()
			.replace(/\s+/g, ',')
			.split(',')
			.filter(Boolean);

		for (const ns of split) {
			if (ns[0] === '-') {
				createDebug.skips.push(ns.slice(1));
			} else {
				createDebug.names.push(ns);
			}
		}
	}

	/**
	 * Checks if the given string matches a namespace template, honoring
	 * asterisks as wildcards.
	 *
	 * @param {String} search
	 * @param {String} template
	 * @return {Boolean}
	 */
	function matchesTemplate(search, template) {
		let searchIndex = 0;
		let templateIndex = 0;
		let starIndex = -1;
		let matchIndex = 0;

		while (searchIndex < search.length) {
			if (templateIndex < template.length && (template[templateIndex] === search[searchIndex] || template[templateIndex] === '*')) {
				// Match character or proceed with wildcard
				if (template[templateIndex] === '*') {
					starIndex = templateIndex;
					matchIndex = searchIndex;
					templateIndex++; // Skip the '*'
				} else {
					searchIndex++;
					templateIndex++;
				}
			} else if (starIndex !== -1) { // eslint-disable-line no-negated-condition
				// Backtrack to the last '*' and try to match more characters
				templateIndex = starIndex + 1;
				matchIndex++;
				searchIndex = matchIndex;
			} else {
				return false; // No match
			}
		}

		// Handle trailing '*' in template
		while (templateIndex < template.length && template[templateIndex] === '*') {
			templateIndex++;
		}

		return templateIndex === template.length;
	}

	/**
	* Disable debug output.
	*
	* @return {String} namespaces
	* @api public
	*/
	function disable() {
		const namespaces = [
			...createDebug.names,
			...createDebug.skips.map(namespace => '-' + namespace)
		].join(',');
		createDebug.enable('');
		return namespaces;
	}

	/**
	* Returns true if the given mode name is enabled, false otherwise.
	*
	* @param {String} name
	* @return {Boolean}
	* @api public
	*/
	function enabled(name) {
		for (const skip of createDebug.skips) {
			if (matchesTemplate(name, skip)) {
				return false;
			}
		}

		for (const ns of createDebug.names) {
			if (matchesTemplate(name, ns)) {
				return true;
			}
		}

		return false;
	}

	/**
	* Coerce `val`.
	*
	* @param {Mixed} val
	* @return {Mixed}
	* @api private
	*/
	function coerce(val) {
		if (val instanceof Error) {
			return val.stack || val.message;
		}
		return val;
	}

	/**
	* XXX DO NOT USE. This is a temporary stub function.
	* XXX It WILL be removed in the next major release.
	*/
	function destroy() {
		console.warn('Instance method `debug.destroy()` is deprecated and no longer does anything. It will be removed in the next major version of `debug`.');
	}

	createDebug.enable(createDebug.load());

	return createDebug;
}

module.exports = setup;


/***/ }),

/***/ 53465:
/***/ (() => {

"use strict";

/**
 * @license Angular
 * (c) 2010-2026 Google LLC. https://angular.dev/
 * License: MIT
 */
var __defProp = Object.defineProperty;
var __getOwnPropSymbols = Object.getOwnPropertySymbols;
var __hasOwnProp = Object.prototype.hasOwnProperty;
var __propIsEnum = Object.prototype.propertyIsEnumerable;
var __defNormalProp = (obj, key, value) => key in obj ? __defProp(obj, key, { enumerable: true, configurable: true, writable: true, value }) : obj[key] = value;
var __spreadValues = (a, b) => {
  for (var prop in b || (b = {}))
    if (__hasOwnProp.call(b, prop))
      __defNormalProp(a, prop, b[prop]);
  if (__getOwnPropSymbols)
    for (var prop of __getOwnPropSymbols(b)) {
      if (__propIsEnum.call(b, prop))
        __defNormalProp(a, prop, b[prop]);
    }
  return a;
};
var __publicField = (obj, key, value) => {
  __defNormalProp(obj, typeof key !== "symbol" ? key + "" : key, value);
  return value;
};

// packages/zone.js/lib/zone-impl.js
var global = globalThis;
function __symbol__(name) {
  const rawPrefix = global["__Zone_symbol_prefix"];
  const symbolPrefix = typeof rawPrefix === "string" ? rawPrefix : "__zone_symbol__";
  return symbolPrefix + name;
}
function initZone() {
  const performance = global["performance"];
  function mark(name) {
    performance && performance["mark"] && performance["mark"](name);
  }
  function performanceMeasure(name, label) {
    performance && performance["measure"] && performance["measure"](name, label);
  }
  mark("Zone");
  const _ZoneImpl = class _ZoneImpl {
    constructor(parent, zoneSpec) {
      __publicField(this, "_parent");
      __publicField(this, "_name");
      __publicField(this, "_properties");
      __publicField(this, "_zoneDelegate");
      this._parent = parent;
      this._name = zoneSpec ? zoneSpec.name || "unnamed" : "<root>";
      this._properties = zoneSpec && zoneSpec.properties || {};
      this._zoneDelegate = new _ZoneDelegate(this, this._parent && this._parent._zoneDelegate, zoneSpec);
    }
    static assertZonePatched() {
      if (global["Promise"] !== patches["ZoneAwarePromise"]) {
        throw new Error("Zone.js has detected that ZoneAwarePromise `(window|global).Promise` has been overwritten.\nMost likely cause is that a Promise polyfill has been loaded after Zone.js (Polyfilling Promise api is not necessary when zone.js is loaded. If you must load one, do so before loading zone.js.)");
      }
    }
    static get root() {
      let zone = _ZoneImpl.current;
      while (zone.parent) {
        zone = zone.parent;
      }
      return zone;
    }
    static get current() {
      return _currentZoneFrame.zone;
    }
    static get currentTask() {
      return _currentTask;
    }
    static __load_patch(name, fn, ignoreDuplicate = false) {
      if (Object.hasOwn(patches, name)) {
        const checkDuplicate = global[__symbol__("forceDuplicateZoneCheck")] === true;
        if (!ignoreDuplicate && checkDuplicate) {
          throw Error("Already loaded patch: " + name);
        }
      } else if (!global["__Zone_disable_" + name]) {
        const perfName = "Zone:" + name;
        mark(perfName);
        patches[name] = fn(global, _ZoneImpl, _api);
        performanceMeasure(perfName, perfName);
      }
    }
    get parent() {
      return this._parent;
    }
    get name() {
      return this._name;
    }
    get(key) {
      const zone = this.getZoneWith(key);
      if (zone)
        return zone._properties[key];
    }
    getZoneWith(key) {
      let current = this;
      while (current) {
        if (Object.hasOwn(current._properties, key)) {
          return current;
        }
        current = current._parent;
      }
      return null;
    }
    fork(zoneSpec) {
      if (!zoneSpec)
        throw new Error("ZoneSpec required!");
      return this._zoneDelegate.fork(this, zoneSpec);
    }
    wrap(callback, source) {
      if (typeof callback !== "function") {
        throw new Error("Expecting function got: " + callback);
      }
      const _callback = this._zoneDelegate.intercept(this, callback, source);
      const zone = this;
      return function() {
        return zone.runGuarded(_callback, this, arguments, source);
      };
    }
    run(callback, applyThis, applyArgs, source) {
      _currentZoneFrame = { parent: _currentZoneFrame, zone: this };
      try {
        return this._zoneDelegate.invoke(this, callback, applyThis, applyArgs, source);
      } finally {
        _currentZoneFrame = _currentZoneFrame.parent;
      }
    }
    runGuarded(callback, applyThis = null, applyArgs, source) {
      _currentZoneFrame = { parent: _currentZoneFrame, zone: this };
      try {
        try {
          return this._zoneDelegate.invoke(this, callback, applyThis, applyArgs, source);
        } catch (error) {
          if (this._zoneDelegate.handleError(this, error)) {
            throw error;
          }
        }
      } finally {
        _currentZoneFrame = _currentZoneFrame.parent;
      }
    }
    runTask(task, applyThis, applyArgs) {
      if (task.zone != this) {
        throw new Error("A task can only be run in the zone of creation! (Creation: " + (task.zone || NO_ZONE).name + "; Execution: " + this.name + ")");
      }
      const zoneTask = task;
      const { type, data: { isPeriodic = false, isRefreshable = false } = {} } = task;
      if (task.state === notScheduled && (type === eventTask || type === macroTask)) {
        return;
      }
      const reEntryGuard = task.state != running;
      reEntryGuard && zoneTask._transitionTo(running, scheduled);
      const previousTask = _currentTask;
      _currentTask = zoneTask;
      _currentZoneFrame = { parent: _currentZoneFrame, zone: this };
      try {
        if (type == macroTask && task.data && !isPeriodic && !isRefreshable) {
          task.cancelFn = void 0;
        }
        try {
          return this._zoneDelegate.invokeTask(this, zoneTask, applyThis, applyArgs);
        } catch (error) {
          if (this._zoneDelegate.handleError(this, error)) {
            throw error;
          }
        }
      } finally {
        const state = task.state;
        if (state !== notScheduled && state !== unknown) {
          if (type == eventTask || isPeriodic || isRefreshable && state === scheduling) {
            reEntryGuard && zoneTask._transitionTo(scheduled, running, scheduling);
          } else {
            const zoneDelegates = zoneTask._zoneDelegates;
            this._updateTaskCount(zoneTask, -1);
            reEntryGuard && zoneTask._transitionTo(notScheduled, running, notScheduled);
            if (isRefreshable) {
              zoneTask._zoneDelegates = zoneDelegates;
            }
          }
        }
        _currentZoneFrame = _currentZoneFrame.parent;
        _currentTask = previousTask;
      }
    }
    scheduleTask(task) {
      if (task.zone && task.zone !== this) {
        let newZone = this;
        while (newZone) {
          if (newZone === task.zone) {
            throw Error(`can not reschedule task to ${this.name} which is descendants of the original zone ${task.zone.name}`);
          }
          newZone = newZone.parent;
        }
      }
      task._transitionTo(scheduling, notScheduled);
      const zoneDelegates = [];
      task._zoneDelegates = zoneDelegates;
      task._zone = this;
      try {
        task = this._zoneDelegate.scheduleTask(this, task);
      } catch (err) {
        task._transitionTo(unknown, scheduling, notScheduled);
        this._zoneDelegate.handleError(this, err);
        throw err;
      }
      if (task._zoneDelegates === zoneDelegates) {
        this._updateTaskCount(task, 1);
      }
      if (task.state == scheduling) {
        task._transitionTo(scheduled, scheduling);
      }
      return task;
    }
    scheduleMicroTask(source, callback, data, customSchedule) {
      return this.scheduleTask(new ZoneTask(microTask, source, callback, data, customSchedule, void 0));
    }
    scheduleMacroTask(source, callback, data, customSchedule, customCancel) {
      return this.scheduleTask(new ZoneTask(macroTask, source, callback, data, customSchedule, customCancel));
    }
    scheduleEventTask(source, callback, data, customSchedule, customCancel) {
      return this.scheduleTask(new ZoneTask(eventTask, source, callback, data, customSchedule, customCancel));
    }
    cancelTask(task) {
      if (task.zone != this)
        throw new Error("A task can only be cancelled in the zone of creation! (Creation: " + (task.zone || NO_ZONE).name + "; Execution: " + this.name + ")");
      if (task.state !== scheduled && task.state !== running) {
        return;
      }
      task._transitionTo(canceling, scheduled, running);
      try {
        this._zoneDelegate.cancelTask(this, task);
      } catch (err) {
        task._transitionTo(unknown, canceling);
        this._zoneDelegate.handleError(this, err);
        throw err;
      }
      this._updateTaskCount(task, -1);
      task._transitionTo(notScheduled, canceling);
      task.runCount = -1;
      return task;
    }
    _updateTaskCount(task, count) {
      const zoneDelegates = task._zoneDelegates;
      if (count == -1) {
        task._zoneDelegates = null;
      }
      for (let i = 0; i < zoneDelegates.length; i++) {
        zoneDelegates[i]._updateTaskCount(task.type, count);
      }
    }
  };
  __publicField(_ZoneImpl, "__symbol__", __symbol__);
  let ZoneImpl = _ZoneImpl;
  const DELEGATE_ZS = {
    name: "",
    onHasTask: (delegate, _, target, hasTaskState) => delegate.hasTask(target, hasTaskState),
    onScheduleTask: (delegate, _, target, task) => delegate.scheduleTask(target, task),
    onInvokeTask: (delegate, _, target, task, applyThis, applyArgs) => delegate.invokeTask(target, task, applyThis, applyArgs),
    onCancelTask: (delegate, _, target, task) => delegate.cancelTask(target, task)
  };
  class _ZoneDelegate {
    constructor(zone, parentDelegate, zoneSpec) {
      __publicField(this, "_zone");
      __publicField(this, "_taskCounts", {
        "microTask": 0,
        "macroTask": 0,
        "eventTask": 0
      });
      __publicField(this, "_forkDlgt");
      __publicField(this, "_forkZS");
      __publicField(this, "_forkCurrZone");
      __publicField(this, "_interceptDlgt");
      __publicField(this, "_interceptZS");
      __publicField(this, "_interceptCurrZone");
      __publicField(this, "_invokeDlgt");
      __publicField(this, "_invokeZS");
      __publicField(this, "_invokeCurrZone");
      __publicField(this, "_handleErrorDlgt");
      __publicField(this, "_handleErrorZS");
      __publicField(this, "_handleErrorCurrZone");
      __publicField(this, "_scheduleTaskDlgt");
      __publicField(this, "_scheduleTaskZS");
      __publicField(this, "_scheduleTaskCurrZone");
      __publicField(this, "_invokeTaskDlgt");
      __publicField(this, "_invokeTaskZS");
      __publicField(this, "_invokeTaskCurrZone");
      __publicField(this, "_cancelTaskDlgt");
      __publicField(this, "_cancelTaskZS");
      __publicField(this, "_cancelTaskCurrZone");
      __publicField(this, "_hasTaskDlgt");
      __publicField(this, "_hasTaskDlgtOwner");
      __publicField(this, "_hasTaskZS");
      __publicField(this, "_hasTaskCurrZone");
      this._zone = zone;
      this._forkZS = zoneSpec && (zoneSpec && zoneSpec.onFork ? zoneSpec : parentDelegate._forkZS);
      this._forkDlgt = zoneSpec && (zoneSpec.onFork ? parentDelegate : parentDelegate._forkDlgt);
      this._forkCurrZone = zoneSpec && (zoneSpec.onFork ? this._zone : parentDelegate._forkCurrZone);
      this._interceptZS = zoneSpec && (zoneSpec.onIntercept ? zoneSpec : parentDelegate._interceptZS);
      this._interceptDlgt = zoneSpec && (zoneSpec.onIntercept ? parentDelegate : parentDelegate._interceptDlgt);
      this._interceptCurrZone = zoneSpec && (zoneSpec.onIntercept ? this._zone : parentDelegate._interceptCurrZone);
      this._invokeZS = zoneSpec && (zoneSpec.onInvoke ? zoneSpec : parentDelegate._invokeZS);
      this._invokeDlgt = zoneSpec && (zoneSpec.onInvoke ? parentDelegate : parentDelegate._invokeDlgt);
      this._invokeCurrZone = zoneSpec && (zoneSpec.onInvoke ? this._zone : parentDelegate._invokeCurrZone);
      this._handleErrorZS = zoneSpec && (zoneSpec.onHandleError ? zoneSpec : parentDelegate._handleErrorZS);
      this._handleErrorDlgt = zoneSpec && (zoneSpec.onHandleError ? parentDelegate : parentDelegate._handleErrorDlgt);
      this._handleErrorCurrZone = zoneSpec && (zoneSpec.onHandleError ? this._zone : parentDelegate._handleErrorCurrZone);
      this._scheduleTaskZS = zoneSpec && (zoneSpec.onScheduleTask ? zoneSpec : parentDelegate._scheduleTaskZS);
      this._scheduleTaskDlgt = zoneSpec && (zoneSpec.onScheduleTask ? parentDelegate : parentDelegate._scheduleTaskDlgt);
      this._scheduleTaskCurrZone = zoneSpec && (zoneSpec.onScheduleTask ? this._zone : parentDelegate._scheduleTaskCurrZone);
      this._invokeTaskZS = zoneSpec && (zoneSpec.onInvokeTask ? zoneSpec : parentDelegate._invokeTaskZS);
      this._invokeTaskDlgt = zoneSpec && (zoneSpec.onInvokeTask ? parentDelegate : parentDelegate._invokeTaskDlgt);
      this._invokeTaskCurrZone = zoneSpec && (zoneSpec.onInvokeTask ? this._zone : parentDelegate._invokeTaskCurrZone);
      this._cancelTaskZS = zoneSpec && (zoneSpec.onCancelTask ? zoneSpec : parentDelegate._cancelTaskZS);
      this._cancelTaskDlgt = zoneSpec && (zoneSpec.onCancelTask ? parentDelegate : parentDelegate._cancelTaskDlgt);
      this._cancelTaskCurrZone = zoneSpec && (zoneSpec.onCancelTask ? this._zone : parentDelegate._cancelTaskCurrZone);
      this._hasTaskZS = null;
      this._hasTaskDlgt = null;
      this._hasTaskDlgtOwner = null;
      this._hasTaskCurrZone = null;
      const zoneSpecHasTask = zoneSpec && zoneSpec.onHasTask;
      const parentHasTask = parentDelegate && parentDelegate._hasTaskZS;
      if (zoneSpecHasTask || parentHasTask) {
        this._hasTaskZS = zoneSpecHasTask ? zoneSpec : DELEGATE_ZS;
        this._hasTaskDlgt = parentDelegate;
        this._hasTaskDlgtOwner = this;
        this._hasTaskCurrZone = this._zone;
        if (!zoneSpec.onScheduleTask) {
          this._scheduleTaskZS = DELEGATE_ZS;
          this._scheduleTaskDlgt = parentDelegate;
          this._scheduleTaskCurrZone = this._zone;
        }
        if (!zoneSpec.onInvokeTask) {
          this._invokeTaskZS = DELEGATE_ZS;
          this._invokeTaskDlgt = parentDelegate;
          this._invokeTaskCurrZone = this._zone;
        }
        if (!zoneSpec.onCancelTask) {
          this._cancelTaskZS = DELEGATE_ZS;
          this._cancelTaskDlgt = parentDelegate;
          this._cancelTaskCurrZone = this._zone;
        }
      }
    }
    get zone() {
      return this._zone;
    }
    fork(targetZone, zoneSpec) {
      return this._forkZS ? this._forkZS.onFork(this._forkDlgt, this.zone, targetZone, zoneSpec) : new ZoneImpl(targetZone, zoneSpec);
    }
    intercept(targetZone, callback, source) {
      return this._interceptZS ? this._interceptZS.onIntercept(this._interceptDlgt, this._interceptCurrZone, targetZone, callback, source) : callback;
    }
    invoke(targetZone, callback, applyThis, applyArgs, source) {
      return this._invokeZS ? this._invokeZS.onInvoke(this._invokeDlgt, this._invokeCurrZone, targetZone, callback, applyThis, applyArgs, source) : callback.apply(applyThis, applyArgs);
    }
    handleError(targetZone, error) {
      return this._handleErrorZS ? this._handleErrorZS.onHandleError(this._handleErrorDlgt, this._handleErrorCurrZone, targetZone, error) : true;
    }
    scheduleTask(targetZone, task) {
      let returnTask = task;
      if (this._scheduleTaskZS) {
        if (this._hasTaskZS) {
          returnTask._zoneDelegates.push(this._hasTaskDlgtOwner);
        }
        returnTask = this._scheduleTaskZS.onScheduleTask(this._scheduleTaskDlgt, this._scheduleTaskCurrZone, targetZone, task);
        if (!returnTask)
          returnTask = task;
      } else {
        if (task.scheduleFn) {
          task.scheduleFn(task);
        } else if (task.type == microTask) {
          scheduleMicroTask(task);
        } else {
          throw new Error("Task is missing scheduleFn.");
        }
      }
      return returnTask;
    }
    invokeTask(targetZone, task, applyThis, applyArgs) {
      return this._invokeTaskZS ? this._invokeTaskZS.onInvokeTask(this._invokeTaskDlgt, this._invokeTaskCurrZone, targetZone, task, applyThis, applyArgs) : task.callback.apply(applyThis, applyArgs);
    }
    cancelTask(targetZone, task) {
      let value;
      if (this._cancelTaskZS) {
        value = this._cancelTaskZS.onCancelTask(this._cancelTaskDlgt, this._cancelTaskCurrZone, targetZone, task);
      } else {
        if (!task.cancelFn) {
          throw Error("Task is not cancelable");
        }
        value = task.cancelFn(task);
      }
      return value;
    }
    hasTask(targetZone, isEmpty) {
      try {
        this._hasTaskZS && this._hasTaskZS.onHasTask(this._hasTaskDlgt, this._hasTaskCurrZone, targetZone, isEmpty);
      } catch (err) {
        this.handleError(targetZone, err);
      }
    }
    _updateTaskCount(type, count) {
      const counts = this._taskCounts;
      const prev = counts[type];
      const next = counts[type] = prev + count;
      if (next < 0) {
        throw new Error("More tasks executed then were scheduled.");
      }
      if (prev == 0 || next == 0) {
        const isEmpty = {
          microTask: counts["microTask"] > 0,
          macroTask: counts["macroTask"] > 0,
          eventTask: counts["eventTask"] > 0,
          change: type
        };
        this.hasTask(this._zone, isEmpty);
      }
    }
  }
  class ZoneTask {
    constructor(type, source, callback, options, scheduleFn, cancelFn) {
      __publicField(this, "type");
      __publicField(this, "source");
      __publicField(this, "invoke");
      __publicField(this, "callback");
      __publicField(this, "data");
      __publicField(this, "scheduleFn");
      __publicField(this, "cancelFn");
      __publicField(this, "_zone", null);
      __publicField(this, "runCount", 0);
      __publicField(this, "_zoneDelegates", null);
      __publicField(this, "_state", "notScheduled");
      this.type = type;
      this.source = source;
      this.data = options;
      this.scheduleFn = scheduleFn;
      this.cancelFn = cancelFn;
      if (!callback) {
        throw new Error("callback is not defined");
      }
      this.callback = callback;
      const self2 = this;
      if (type === eventTask && options && options.useG) {
        this.invoke = ZoneTask.invokeTask;
      } else {
        this.invoke = function() {
          return ZoneTask.invokeTask.call(global, self2, this, arguments);
        };
      }
    }
    static invokeTask(task, target, args) {
      if (!task) {
        task = this;
      }
      _numberOfNestedTaskFrames++;
      try {
        task.runCount++;
        return task.zone.runTask(task, target, args);
      } finally {
        try {
          if (_numberOfNestedTaskFrames === 1 && !global[enableNativeMicrotaskDraining]) {
            drainMicroTaskQueueSynchronously();
          }
        } finally {
          _numberOfNestedTaskFrames--;
        }
      }
    }
    get zone() {
      return this._zone;
    }
    get state() {
      return this._state;
    }
    cancelScheduleRequest() {
      this._transitionTo(notScheduled, scheduling);
    }
    _transitionTo(toState, fromState1, fromState2) {
      if (this._state === fromState1 || this._state === fromState2) {
        this._state = toState;
        if (toState == notScheduled) {
          this._zoneDelegates = null;
        }
      } else {
        throw new Error(`${this.type} '${this.source}': can not transition to '${toState}', expecting state '${fromState1}'${fromState2 ? " or '" + fromState2 + "'" : ""}, was '${this._state}'.`);
      }
    }
    toString() {
      if (this.data && typeof this.data.handleId !== "undefined") {
        return this.data.handleId.toString();
      } else {
        return Object.prototype.toString.call(this);
      }
    }
    // add toJSON method to prevent cyclic error when
    // call JSON.stringify(zoneTask)
    toJSON() {
      return {
        type: this.type,
        state: this.state,
        source: this.source,
        zone: this.zone.name,
        runCount: this.runCount
      };
    }
  }
  const symbolSetTimeout = __symbol__("setTimeout");
  const symbolPromise = __symbol__("Promise");
  const symbolThen = __symbol__("then");
  const enableNativeMicrotaskDraining = __symbol__("enable_native_microtask_draining");
  let _microTaskQueue = [];
  let _isDrainingMicrotaskQueue = false;
  let nativeMicroTaskQueuePromise;
  function nativeScheduleMicroTask(func) {
    var _a;
    if (!nativeMicroTaskQueuePromise && global[symbolPromise]) {
      nativeMicroTaskQueuePromise = global[symbolPromise].resolve(0);
    }
    if (nativeMicroTaskQueuePromise) {
      const thenFn = (_a = nativeMicroTaskQueuePromise[symbolThen]) != null ? _a : nativeMicroTaskQueuePromise["then"];
      thenFn.call(nativeMicroTaskQueuePromise, func);
    } else {
      global[symbolSetTimeout](func, 0);
    }
  }
  function scheduleMicroTask(task) {
    const isNativeDrainingEnabled = global[enableNativeMicrotaskDraining];
    const shouldDrainWithNative = isNativeDrainingEnabled && _microTaskQueue.length === 0 && !_isDrainingMicrotaskQueue;
    const shouldDrainWithoutNative = !isNativeDrainingEnabled && _numberOfNestedTaskFrames === 0 && _microTaskQueue.length === 0;
    if (shouldDrainWithNative || shouldDrainWithoutNative) {
      nativeScheduleMicroTask(drainMicroTaskQueueSynchronously);
    }
    if (task) {
      _microTaskQueue.push(task);
    }
  }
  function drainMicroTaskQueueSynchronously() {
    if (_isDrainingMicrotaskQueue) {
      return;
    }
    _isDrainingMicrotaskQueue = true;
    try {
      while (_microTaskQueue.length) {
        const queue = _microTaskQueue;
        _microTaskQueue = [];
        for (const task of queue) {
          try {
            task.zone.runTask(task, null, null);
          } catch (error) {
            _api.onUnhandledError(error);
          }
        }
      }
    } finally {
      if (global[enableNativeMicrotaskDraining]) {
        _isDrainingMicrotaskQueue = false;
        _api.microtaskDrainDone();
      } else {
        try {
          _api.microtaskDrainDone();
        } finally {
          _isDrainingMicrotaskQueue = false;
        }
      }
    }
  }
  const NO_ZONE = { name: "NO ZONE" };
  const notScheduled = "notScheduled", scheduling = "scheduling", scheduled = "scheduled", running = "running", canceling = "canceling", unknown = "unknown";
  const microTask = "microTask", macroTask = "macroTask", eventTask = "eventTask";
  const patches = /* @__PURE__ */ Object.create(null);
  const _api = {
    symbol: __symbol__,
    currentZoneFrame: () => _currentZoneFrame,
    onUnhandledError: noop,
    microtaskDrainDone: noop,
    scheduleMicroTask,
    showUncaughtError: () => !ZoneImpl[__symbol__("ignoreConsoleErrorUncaughtError")],
    patchEventTarget: () => [],
    patchOnProperties: noop,
    patchMethod: () => noop,
    bindArguments: () => [],
    patchThen: () => noop,
    patchMacroTask: () => noop,
    patchEventPrototype: () => noop,
    getGlobalObjects: () => void 0,
    ObjectDefineProperty: () => noop,
    ObjectGetOwnPropertyDescriptor: () => void 0,
    ObjectCreate: () => void 0,
    ArraySlice: () => [],
    patchClass: () => noop,
    wrapWithCurrentZone: () => noop,
    filterProperties: () => [],
    attachOriginToPatched: () => noop,
    _redefineProperty: () => noop,
    patchCallbacks: () => noop,
    nativeScheduleMicroTask
  };
  let _currentZoneFrame = { parent: null, zone: new ZoneImpl(null, null) };
  let _currentTask = null;
  let _numberOfNestedTaskFrames = 0;
  function noop() {
  }
  performanceMeasure("Zone", "Zone");
  return ZoneImpl;
}

// packages/zone.js/lib/zone.js
function loadZone() {
  var _a;
  const global2 = globalThis;
  const checkDuplicate = global2[__symbol__("forceDuplicateZoneCheck")] === true;
  if (global2["Zone"] && (checkDuplicate || typeof global2["Zone"].__symbol__ !== "function")) {
    throw new Error("Zone already loaded.");
  }
  (_a = global2["Zone"]) != null ? _a : global2["Zone"] = initZone();
  return global2["Zone"];
}

// packages/zone.js/lib/common/utils.js
var ObjectGetOwnPropertyDescriptor = Object.getOwnPropertyDescriptor;
var ObjectDefineProperty = Object.defineProperty;
var ObjectGetPrototypeOf = Object.getPrototypeOf;
var ObjectCreate = Object.create;
var ArraySlice = Array.prototype.slice;
var ADD_EVENT_LISTENER_STR = "addEventListener";
var REMOVE_EVENT_LISTENER_STR = "removeEventListener";
var ZONE_SYMBOL_ADD_EVENT_LISTENER = __symbol__(ADD_EVENT_LISTENER_STR);
var ZONE_SYMBOL_REMOVE_EVENT_LISTENER = __symbol__(REMOVE_EVENT_LISTENER_STR);
var TRUE_STR = "true";
var FALSE_STR = "false";
var ZONE_SYMBOL_PREFIX = __symbol__("");
function wrapWithCurrentZone(callback, source) {
  return Zone.current.wrap(callback, source);
}
function scheduleMacroTaskWithCurrentZone(source, callback, data, customSchedule, customCancel) {
  return Zone.current.scheduleMacroTask(source, callback, data, customSchedule, customCancel);
}
var zoneSymbol = __symbol__;
var isWindowExists = typeof window !== "undefined";
var internalWindow = isWindowExists ? window : void 0;
var _global = isWindowExists && internalWindow || globalThis;
var REMOVE_ATTRIBUTE = "removeAttribute";
function bindArguments(args, source) {
  for (let i = args.length - 1; i >= 0; i--) {
    if (typeof args[i] === "function") {
      args[i] = wrapWithCurrentZone(args[i], source + "_" + i);
    }
  }
  return args;
}
function patchPrototype(prototype, fnNames) {
  const source = prototype.constructor["name"];
  for (let i = 0; i < fnNames.length; i++) {
    const name = fnNames[i];
    const delegate = prototype[name];
    if (delegate) {
      const prototypeDesc = ObjectGetOwnPropertyDescriptor(prototype, name);
      if (!isPropertyWritable(prototypeDesc)) {
        continue;
      }
      prototype[name] = ((delegate2) => {
        const patched = function() {
          return delegate2.apply(this, bindArguments(arguments, source + "." + name));
        };
        attachOriginToPatched(patched, delegate2);
        return patched;
      })(delegate);
    }
  }
}
function isPropertyWritable(propertyDesc) {
  if (!propertyDesc) {
    return true;
  }
  if (propertyDesc.writable === false) {
    return false;
  }
  return !(typeof propertyDesc.get === "function" && typeof propertyDesc.set === "undefined");
}
var isWebWorker = typeof WorkerGlobalScope !== "undefined" && self instanceof WorkerGlobalScope;
var isNode = !("nw" in _global) && typeof _global.process !== "undefined" && _global.process.toString() === "[object process]";
var isBrowser = !isNode && !isWebWorker && !!(isWindowExists && internalWindow["HTMLElement"]);
var isMix = typeof _global.process !== "undefined" && _global.process.toString() === "[object process]" && !isWebWorker && !!(isWindowExists && internalWindow["HTMLElement"]);
var zoneSymbolEventNames = /* @__PURE__ */ Object.create(null);
var enableBeforeunloadSymbol = zoneSymbol("enable_beforeunload");
var wrapFn = function(event) {
  event = event || _global.event;
  if (!event) {
    return;
  }
  let eventNameSymbol = zoneSymbolEventNames[event.type];
  if (!eventNameSymbol) {
    eventNameSymbol = zoneSymbolEventNames[event.type] = zoneSymbol("ON_PROPERTY" + event.type);
  }
  const target = this || event.target || _global;
  const listener = target[eventNameSymbol];
  let result;
  if (isBrowser && target === internalWindow && event.type === "error") {
    const errorEvent = event;
    result = listener && listener.call(this, errorEvent.message, errorEvent.filename, errorEvent.lineno, errorEvent.colno, errorEvent.error);
    if (result === true) {
      event.preventDefault();
    }
  } else {
    result = listener && listener.apply(this, arguments);
    if (
      // https://github.com/angular/angular/issues/47579
      // https://www.w3.org/TR/2011/WD-html5-20110525/history.html#beforeunloadevent
      // This is the only specific case we should check for. The spec defines that the
      // `returnValue` attribute represents the message to show the user. When the event
      // is created, this attribute must be set to the empty string.
      event.type === "beforeunload" && // To prevent any breaking changes resulting from this change, given that
      // it was already causing a significant number of failures in G3, we have hidden
      // that behavior behind a global configuration flag. Consumers can enable this
      // flag explicitly if they want the `beforeunload` event to be handled as defined
      // in the specification.
      _global[enableBeforeunloadSymbol] && // The IDL event definition is `attribute DOMString returnValue`, so we check whether
      // `typeof result` is a string.
      typeof result === "string"
    ) {
      event.returnValue = result;
    } else if (result != void 0 && !result) {
      event.preventDefault();
    }
  }
  return result;
};
function patchProperty(obj, prop, prototype) {
  let desc = ObjectGetOwnPropertyDescriptor(obj, prop);
  if (!desc && prototype) {
    const prototypeDesc = ObjectGetOwnPropertyDescriptor(prototype, prop);
    if (prototypeDesc) {
      desc = { enumerable: true, configurable: true };
    }
  }
  if (!desc || !desc.configurable) {
    return;
  }
  const onPropPatchedSymbol = zoneSymbol("on" + prop + "patched");
  if (Object.hasOwn(obj, onPropPatchedSymbol) && obj[onPropPatchedSymbol]) {
    return;
  }
  delete desc.writable;
  delete desc.value;
  const originalDescGet = desc.get;
  const originalDescSet = desc.set;
  const eventName = prop.slice(2);
  let eventNameSymbol = zoneSymbolEventNames[eventName];
  if (!eventNameSymbol) {
    eventNameSymbol = zoneSymbolEventNames[eventName] = zoneSymbol("ON_PROPERTY" + eventName);
  }
  desc.set = function(newValue) {
    let target = this;
    if (!target && obj === _global) {
      target = _global;
    }
    if (!target) {
      return;
    }
    const previousValue = target[eventNameSymbol];
    if (typeof previousValue === "function") {
      target.removeEventListener(eventName, wrapFn);
    }
    originalDescSet == null ? void 0 : originalDescSet.call(target, null);
    target[eventNameSymbol] = newValue;
    if (typeof newValue === "function") {
      target.addEventListener(eventName, wrapFn, false);
    }
  };
  desc.get = function() {
    let target = this;
    if (!target && obj === _global) {
      target = _global;
    }
    if (!target) {
      return null;
    }
    const listener = target[eventNameSymbol];
    if (listener) {
      return listener;
    } else if (originalDescGet) {
      let value = originalDescGet.call(this);
      if (value) {
        desc.set.call(this, value);
        if (typeof target[REMOVE_ATTRIBUTE] === "function") {
          target.removeAttribute(prop);
        }
        return value;
      }
    }
    return null;
  };
  ObjectDefineProperty(obj, prop, desc);
  obj[onPropPatchedSymbol] = true;
}
function patchOnProperties(obj, properties, prototype) {
  if (properties) {
    for (let i = 0; i < properties.length; i++) {
      patchProperty(obj, "on" + properties[i], prototype);
    }
  } else {
    const onProperties = [];
    for (const prop in obj) {
      if (prop.slice(0, 2) == "on") {
        onProperties.push(prop);
      }
    }
    for (let j = 0; j < onProperties.length; j++) {
      patchProperty(obj, onProperties[j], prototype);
    }
  }
}
var originalInstanceKey = zoneSymbol("originalInstance");
function patchClass(className) {
  const OriginalClass = _global[className];
  if (!OriginalClass)
    return;
  _global[zoneSymbol(className)] = OriginalClass;
  _global[className] = function() {
    const a = bindArguments(arguments, className);
    switch (a.length) {
      case 0:
        this[originalInstanceKey] = new OriginalClass();
        break;
      case 1:
        this[originalInstanceKey] = new OriginalClass(a[0]);
        break;
      case 2:
        this[originalInstanceKey] = new OriginalClass(a[0], a[1]);
        break;
      case 3:
        this[originalInstanceKey] = new OriginalClass(a[0], a[1], a[2]);
        break;
      case 4:
        this[originalInstanceKey] = new OriginalClass(a[0], a[1], a[2], a[3]);
        break;
      default:
        throw new Error("Arg list too long.");
    }
  };
  attachOriginToPatched(_global[className], OriginalClass);
  const instance = new OriginalClass(function() {
  });
  let prop;
  for (prop in instance) {
    if (className === "XMLHttpRequest" && prop === "responseBlob")
      continue;
    (function(prop2) {
      if (typeof instance[prop2] === "function") {
        _global[className].prototype[prop2] = function() {
          return this[originalInstanceKey][prop2].apply(this[originalInstanceKey], arguments);
        };
      } else {
        ObjectDefineProperty(_global[className].prototype, prop2, {
          set: function(fn) {
            if (typeof fn === "function") {
              this[originalInstanceKey][prop2] = wrapWithCurrentZone(fn, className + "." + prop2);
              attachOriginToPatched(this[originalInstanceKey][prop2], fn);
            } else {
              this[originalInstanceKey][prop2] = fn;
            }
          },
          get: function() {
            return this[originalInstanceKey][prop2];
          }
        });
      }
    })(prop);
  }
  for (prop in OriginalClass) {
    if (prop !== "prototype" && Object.hasOwn(OriginalClass, prop)) {
      _global[className][prop] = OriginalClass[prop];
    }
  }
}
function copySymbolProperties(src, dest) {
  if (typeof Object.getOwnPropertySymbols !== "function") {
    return;
  }
  const symbols = Object.getOwnPropertySymbols(src);
  symbols.forEach((symbol) => {
    const desc = Object.getOwnPropertyDescriptor(src, symbol);
    Object.defineProperty(dest, symbol, {
      get: function() {
        return src[symbol];
      },
      set: function(value) {
        if (desc && (!desc.writable || typeof desc.set !== "function")) {
          return;
        }
        src[symbol] = value;
      },
      enumerable: desc ? desc.enumerable : true,
      configurable: desc ? desc.configurable : true
    });
  });
}
var shouldCopySymbolProperties = false;
function patchMethod(target, name, patchFn) {
  let proto = target;
  while (proto && !Object.hasOwn(proto, name)) {
    proto = ObjectGetPrototypeOf(proto);
  }
  if (!proto && target[name]) {
    proto = target;
  }
  const delegateName = zoneSymbol(name);
  let delegate = null;
  if (proto && (!(delegate = proto[delegateName]) || !Object.hasOwn(proto, delegateName))) {
    delegate = proto[delegateName] = proto[name];
    const desc = proto && ObjectGetOwnPropertyDescriptor(proto, name);
    if (isPropertyWritable(desc)) {
      const patchDelegate = patchFn(delegate, delegateName, name);
      proto[name] = function() {
        return patchDelegate(this, arguments);
      };
      attachOriginToPatched(proto[name], delegate);
      if (shouldCopySymbolProperties) {
        copySymbolProperties(delegate, proto[name]);
      }
    }
  }
  return delegate;
}
function patchMacroTask(obj, funcName, metaCreator) {
  let setNative = null;
  function scheduleTask(task) {
    const data = task.data;
    data.args[data.cbIdx] = function() {
      task.invoke.apply(this, arguments);
    };
    setNative.apply(data.target, data.args);
    return task;
  }
  setNative = patchMethod(obj, funcName, (delegate) => function(self2, args) {
    const meta = metaCreator(self2, args);
    if (meta.cbIdx >= 0 && typeof args[meta.cbIdx] === "function") {
      return scheduleMacroTaskWithCurrentZone(meta.name, args[meta.cbIdx], meta, scheduleTask);
    } else {
      return delegate.apply(self2, args);
    }
  });
}
function attachOriginToPatched(patched, original) {
  patched[zoneSymbol("OriginalDelegate")] = original;
}
function isFunction(value) {
  return typeof value === "function";
}
function isNumber(value) {
  return typeof value === "number";
}

// packages/zone.js/lib/common/events.js
var OPTIMIZED_ZONE_EVENT_TASK_DATA = {
  useG: true
};
var zoneSymbolEventNames2 = /* @__PURE__ */ Object.create(null);
var globalSources = {};
var EVENT_NAME_SYMBOL_REGX = new RegExp("^" + ZONE_SYMBOL_PREFIX + "(\\w+)(true|false)$");
var IMMEDIATE_PROPAGATION_SYMBOL = zoneSymbol("propagationStopped");
var KNOWN_EVENT_LISTENER_OPTIONS = ["capture", "once", "passive", "signal"];
function prepareEventNames(eventName, eventNameToString) {
  const falseEventName = (eventNameToString ? eventNameToString(eventName) : eventName) + FALSE_STR;
  const trueEventName = (eventNameToString ? eventNameToString(eventName) : eventName) + TRUE_STR;
  const symbol = ZONE_SYMBOL_PREFIX + falseEventName;
  const symbolCapture = ZONE_SYMBOL_PREFIX + trueEventName;
  zoneSymbolEventNames2[eventName] = {
    [FALSE_STR]: symbol,
    [TRUE_STR]: symbolCapture
  };
}
function patchEventTarget(_global2, api, apis, patchOptions) {
  const ADD_EVENT_LISTENER = patchOptions && patchOptions.add || ADD_EVENT_LISTENER_STR;
  const REMOVE_EVENT_LISTENER = patchOptions && patchOptions.rm || REMOVE_EVENT_LISTENER_STR;
  const LISTENERS_EVENT_LISTENER = patchOptions && patchOptions.listeners || "eventListeners";
  const REMOVE_ALL_LISTENERS_EVENT_LISTENER = patchOptions && patchOptions.rmAll || "removeAllListeners";
  const zoneSymbolAddEventListener = zoneSymbol(ADD_EVENT_LISTENER);
  const ADD_EVENT_LISTENER_SOURCE = "." + ADD_EVENT_LISTENER + ":";
  const PREPEND_EVENT_LISTENER = "prependListener";
  const PREPEND_EVENT_LISTENER_SOURCE = "." + PREPEND_EVENT_LISTENER + ":";
  const invokeTask = function(task, target, event) {
    if (task.isRemoved) {
      return;
    }
    const delegate = task.callback;
    if (typeof delegate === "object" && delegate.handleEvent) {
      task.callback = (event2) => delegate.handleEvent(event2);
      task.originalDelegate = delegate;
    }
    let error;
    try {
      task.invoke(task, target, [event]);
    } catch (err) {
      error = err;
    }
    const options = task.options;
    if (options && typeof options === "object" && options.once) {
      const delegate2 = task.originalDelegate ? task.originalDelegate : task.callback;
      target[REMOVE_EVENT_LISTENER].call(target, event.type, delegate2, options);
    }
    return error;
  };
  function globalCallback(context, event, isCapture) {
    event = event || _global2.event;
    if (!event) {
      return;
    }
    const target = context || event.target || _global2;
    const tasks = target[zoneSymbolEventNames2[event.type][isCapture ? TRUE_STR : FALSE_STR]];
    if (tasks) {
      const errors = [];
      if (tasks.length === 1) {
        const err = invokeTask(tasks[0], target, event);
        err && errors.push(err);
      } else {
        const copyTasks = tasks.slice();
        for (let i = 0; i < copyTasks.length; i++) {
          if (event && event[IMMEDIATE_PROPAGATION_SYMBOL] === true) {
            break;
          }
          const err = invokeTask(copyTasks[i], target, event);
          err && errors.push(err);
        }
      }
      if (errors.length === 1) {
        throw errors[0];
      } else {
        for (let i = 0; i < errors.length; i++) {
          const err = errors[i];
          api.nativeScheduleMicroTask(() => {
            throw err;
          });
        }
      }
    }
  }
  const globalZoneAwareCallback = function(event) {
    return globalCallback(this, event, false);
  };
  const globalZoneAwareCaptureCallback = function(event) {
    return globalCallback(this, event, true);
  };
  function patchEventTargetMethods(obj, patchOptions2) {
    if (!obj) {
      return false;
    }
    let useGlobalCallback = true;
    if (patchOptions2 && patchOptions2.useG !== void 0) {
      useGlobalCallback = patchOptions2.useG;
    }
    const validateHandler = patchOptions2 && patchOptions2.vh;
    let checkDuplicate = true;
    if (patchOptions2 && patchOptions2.chkDup !== void 0) {
      checkDuplicate = patchOptions2.chkDup;
    }
    let returnTarget = false;
    if (patchOptions2 && patchOptions2.rt !== void 0) {
      returnTarget = patchOptions2.rt;
    }
    let proto = obj;
    while (proto && !Object.hasOwn(proto, ADD_EVENT_LISTENER)) {
      proto = ObjectGetPrototypeOf(proto);
    }
    if (!proto && obj[ADD_EVENT_LISTENER]) {
      proto = obj;
    }
    if (!proto) {
      return false;
    }
    if (proto[zoneSymbolAddEventListener]) {
      return false;
    }
    const eventNameToString = patchOptions2 && patchOptions2.eventNameToString;
    const taskData = {};
    const nativeAddEventListener = proto[zoneSymbolAddEventListener] = proto[ADD_EVENT_LISTENER];
    const nativeRemoveEventListener = proto[zoneSymbol(REMOVE_EVENT_LISTENER)] = proto[REMOVE_EVENT_LISTENER];
    const nativeListeners = proto[zoneSymbol(LISTENERS_EVENT_LISTENER)] = proto[LISTENERS_EVENT_LISTENER];
    const nativeRemoveAllListeners = proto[zoneSymbol(REMOVE_ALL_LISTENERS_EVENT_LISTENER)] = proto[REMOVE_ALL_LISTENERS_EVENT_LISTENER];
    let nativePrependEventListener;
    if (patchOptions2 && patchOptions2.prepend) {
      nativePrependEventListener = proto[zoneSymbol(patchOptions2.prepend)] = proto[patchOptions2.prepend];
    }
    function buildEventListenerOptions(options, passive) {
      if (!passive) {
        return options;
      }
      if (typeof options === "boolean") {
        return { capture: options, passive: true };
      }
      if (!options) {
        return { passive: true };
      }
      if (typeof options === "object" && options.passive !== false) {
        options.passive = true;
        return options;
      }
      return options;
    }
    const customScheduleGlobal = function(task) {
      if (taskData.isExisting) {
        return;
      }
      return nativeAddEventListener.call(taskData.target, taskData.eventName, taskData.capture ? globalZoneAwareCaptureCallback : globalZoneAwareCallback, taskData.options);
    };
    const customCancelGlobal = function(task) {
      if (!task.isRemoved) {
        const symbolEventNames = zoneSymbolEventNames2[task.eventName];
        let symbolEventName;
        if (symbolEventNames) {
          symbolEventName = symbolEventNames[task.capture ? TRUE_STR : FALSE_STR];
        }
        const existingTasks = symbolEventName && task.target[symbolEventName];
        if (existingTasks) {
          for (let i = 0; i < existingTasks.length; i++) {
            const existingTask = existingTasks[i];
            if (existingTask === task) {
              existingTasks.splice(i, 1);
              task.isRemoved = true;
              if (task.removeAbortListener) {
                task.removeAbortListener();
                task.removeAbortListener = null;
              }
              if (existingTasks.length === 0) {
                task.allRemoved = true;
                task.target[symbolEventName] = null;
              }
              break;
            }
          }
        }
      }
      if (!task.allRemoved) {
        return;
      }
      return nativeRemoveEventListener.call(task.target, task.eventName, task.capture ? globalZoneAwareCaptureCallback : globalZoneAwareCallback, task.options);
    };
    const customScheduleNonGlobal = function(task) {
      return nativeAddEventListener.call(taskData.target, taskData.eventName, task.invoke, taskData.options);
    };
    const customSchedulePrepend = function(task) {
      return nativePrependEventListener.call(taskData.target, taskData.eventName, task.invoke, taskData.options);
    };
    const customCancelNonGlobal = function(task) {
      return nativeRemoveEventListener.call(task.target, task.eventName, task.invoke, task.options);
    };
    const customSchedule = useGlobalCallback ? customScheduleGlobal : customScheduleNonGlobal;
    const customCancel = useGlobalCallback ? customCancelGlobal : customCancelNonGlobal;
    const compareTaskCallbackVsDelegate = function(task, delegate) {
      const typeOfDelegate = typeof delegate;
      return typeOfDelegate === "function" && task.callback === delegate || typeOfDelegate === "object" && task.originalDelegate === delegate;
    };
    const compare = (patchOptions2 == null ? void 0 : patchOptions2.diff) || compareTaskCallbackVsDelegate;
    const unpatchedEvents = Zone[zoneSymbol("UNPATCHED_EVENTS")];
    const passiveEvents = _global2[zoneSymbol("PASSIVE_EVENTS")];
    function copyEventListenerOptions(options) {
      if (typeof options !== "object" || options === null) {
        return options;
      }
      const newOptions = __spreadValues({}, options);
      for (const key of KNOWN_EVENT_LISTENER_OPTIONS) {
        if (!Object.hasOwn(newOptions, key) && key in options) {
          newOptions[key] = options[key];
        }
      }
      return newOptions;
    }
    const makeAddListener = function(nativeListener, addSource, customScheduleFn, customCancelFn, returnTarget2 = false, prepend = false) {
      return function() {
        const target = this || _global2;
        let eventName = arguments[0];
        if (patchOptions2 && patchOptions2.transferEventName) {
          eventName = patchOptions2.transferEventName(eventName);
        }
        let delegate = arguments[1];
        if (!delegate) {
          return nativeListener.apply(this, arguments);
        }
        if (isNode && eventName === "uncaughtException") {
          return nativeListener.apply(this, arguments);
        }
        let isEventListenerObject = false;
        if (typeof delegate !== "function") {
          if (!delegate.handleEvent) {
            return nativeListener.apply(this, arguments);
          }
          isEventListenerObject = true;
        }
        if (validateHandler && !validateHandler(nativeListener, delegate, target, arguments)) {
          return;
        }
        const passive = !!passiveEvents && passiveEvents.indexOf(eventName) !== -1;
        const options = buildEventListenerOptions(copyEventListenerOptions(arguments[2]), passive);
        const signal = options == null ? void 0 : options.signal;
        if (signal == null ? void 0 : signal.aborted) {
          return;
        }
        if (unpatchedEvents) {
          for (let i = 0; i < unpatchedEvents.length; i++) {
            if (eventName === unpatchedEvents[i]) {
              if (passive) {
                return nativeListener.call(target, eventName, delegate, options);
              } else {
                return nativeListener.apply(this, arguments);
              }
            }
          }
        }
        const capture = !options ? false : typeof options === "boolean" ? true : options.capture;
        const once = options && typeof options === "object" ? options.once : false;
        const zone = Zone.current;
        let symbolEventNames = zoneSymbolEventNames2[eventName];
        if (!symbolEventNames) {
          prepareEventNames(eventName, eventNameToString);
          symbolEventNames = zoneSymbolEventNames2[eventName];
        }
        const symbolEventName = symbolEventNames[capture ? TRUE_STR : FALSE_STR];
        let existingTasks = target[symbolEventName];
        let isExisting = false;
        if (existingTasks) {
          isExisting = true;
          if (checkDuplicate) {
            for (let i = 0; i < existingTasks.length; i++) {
              if (compare(existingTasks[i], delegate)) {
                return;
              }
            }
          }
        } else {
          existingTasks = target[symbolEventName] = [];
        }
        let source;
        const constructorName = target.constructor["name"];
        const targetSource = globalSources[constructorName];
        if (targetSource) {
          source = targetSource[eventName];
        }
        if (!source) {
          source = constructorName + addSource + (eventNameToString ? eventNameToString(eventName) : eventName);
        }
        taskData.options = options;
        if (once) {
          taskData.options.once = false;
        }
        taskData.target = target;
        taskData.capture = capture;
        taskData.eventName = eventName;
        taskData.isExisting = isExisting;
        const data = useGlobalCallback ? OPTIMIZED_ZONE_EVENT_TASK_DATA : void 0;
        if (data) {
          data.taskData = taskData;
        }
        if (signal) {
          taskData.options.signal = void 0;
        }
        const task = zone.scheduleEventTask(source, delegate, data, customScheduleFn, customCancelFn);
        if (signal) {
          taskData.options.signal = signal;
          const onAbort = () => task.zone.cancelTask(task);
          nativeListener.call(signal, "abort", onAbort, { once: true });
          task.removeAbortListener = () => signal.removeEventListener("abort", onAbort);
        }
        taskData.target = null;
        if (data) {
          data.taskData = null;
        }
        if (once) {
          taskData.options.once = true;
        }
        if (typeof task.options !== "boolean") {
          task.options = options;
        }
        task.target = target;
        task.capture = capture;
        task.eventName = eventName;
        if (isEventListenerObject) {
          task.originalDelegate = delegate;
        }
        if (!prepend) {
          existingTasks.push(task);
        } else {
          existingTasks.unshift(task);
        }
        if (returnTarget2) {
          return target;
        }
      };
    };
    proto[ADD_EVENT_LISTENER] = makeAddListener(nativeAddEventListener, ADD_EVENT_LISTENER_SOURCE, customSchedule, customCancel, returnTarget);
    if (nativePrependEventListener) {
      proto[PREPEND_EVENT_LISTENER] = makeAddListener(nativePrependEventListener, PREPEND_EVENT_LISTENER_SOURCE, customSchedulePrepend, customCancel, returnTarget, true);
    }
    proto[REMOVE_EVENT_LISTENER] = function() {
      const target = this || _global2;
      let eventName = arguments[0];
      if (patchOptions2 && patchOptions2.transferEventName) {
        eventName = patchOptions2.transferEventName(eventName);
      }
      const options = arguments[2];
      const capture = !options ? false : typeof options === "boolean" ? true : options.capture;
      const delegate = arguments[1];
      if (!delegate) {
        return nativeRemoveEventListener.apply(this, arguments);
      }
      if (validateHandler && !validateHandler(nativeRemoveEventListener, delegate, target, arguments)) {
        return;
      }
      const symbolEventNames = zoneSymbolEventNames2[eventName];
      let symbolEventName;
      if (symbolEventNames) {
        symbolEventName = symbolEventNames[capture ? TRUE_STR : FALSE_STR];
      }
      const existingTasks = symbolEventName && target[symbolEventName];
      if (existingTasks) {
        for (let i = 0; i < existingTasks.length; i++) {
          const existingTask = existingTasks[i];
          if (compare(existingTask, delegate)) {
            existingTasks.splice(i, 1);
            existingTask.isRemoved = true;
            if (existingTasks.length === 0) {
              existingTask.allRemoved = true;
              target[symbolEventName] = null;
              if (!capture && typeof eventName === "string") {
                const onPropertySymbol = ZONE_SYMBOL_PREFIX + "ON_PROPERTY" + eventName;
                target[onPropertySymbol] = null;
              }
            }
            existingTask.zone.cancelTask(existingTask);
            if (returnTarget) {
              return target;
            }
            return;
          }
        }
      }
      return nativeRemoveEventListener.apply(this, arguments);
    };
    proto[LISTENERS_EVENT_LISTENER] = function() {
      const target = this || _global2;
      let eventName = arguments[0];
      if (patchOptions2 && patchOptions2.transferEventName) {
        eventName = patchOptions2.transferEventName(eventName);
      }
      const listeners = [];
      const tasks = findEventTasks(target, eventNameToString ? eventNameToString(eventName) : eventName);
      for (let i = 0; i < tasks.length; i++) {
        const task = tasks[i];
        let delegate = task.originalDelegate ? task.originalDelegate : task.callback;
        listeners.push(delegate);
      }
      return listeners;
    };
    proto[REMOVE_ALL_LISTENERS_EVENT_LISTENER] = function() {
      const target = this || _global2;
      let eventName = arguments[0];
      if (!eventName) {
        const keys = Object.keys(target);
        for (let i = 0; i < keys.length; i++) {
          const prop = keys[i];
          const match = EVENT_NAME_SYMBOL_REGX.exec(prop);
          let evtName = match && match[1];
          if (evtName && evtName !== "removeListener") {
            this[REMOVE_ALL_LISTENERS_EVENT_LISTENER].call(this, evtName);
          }
        }
        this[REMOVE_ALL_LISTENERS_EVENT_LISTENER].call(this, "removeListener");
      } else {
        if (patchOptions2 && patchOptions2.transferEventName) {
          eventName = patchOptions2.transferEventName(eventName);
        }
        const symbolEventNames = zoneSymbolEventNames2[eventName];
        if (symbolEventNames) {
          const symbolEventName = symbolEventNames[FALSE_STR];
          const symbolCaptureEventName = symbolEventNames[TRUE_STR];
          const tasks = target[symbolEventName];
          const captureTasks = target[symbolCaptureEventName];
          if (tasks) {
            const removeTasks = tasks.slice();
            for (let i = 0; i < removeTasks.length; i++) {
              const task = removeTasks[i];
              let delegate = task.originalDelegate ? task.originalDelegate : task.callback;
              this[REMOVE_EVENT_LISTENER].call(this, eventName, delegate, task.options);
            }
          }
          if (captureTasks) {
            const removeTasks = captureTasks.slice();
            for (let i = 0; i < removeTasks.length; i++) {
              const task = removeTasks[i];
              let delegate = task.originalDelegate ? task.originalDelegate : task.callback;
              this[REMOVE_EVENT_LISTENER].call(this, eventName, delegate, task.options);
            }
          }
        }
      }
      if (returnTarget) {
        return this;
      }
    };
    attachOriginToPatched(proto[ADD_EVENT_LISTENER], nativeAddEventListener);
    attachOriginToPatched(proto[REMOVE_EVENT_LISTENER], nativeRemoveEventListener);
    if (nativeRemoveAllListeners) {
      attachOriginToPatched(proto[REMOVE_ALL_LISTENERS_EVENT_LISTENER], nativeRemoveAllListeners);
    }
    if (nativeListeners) {
      attachOriginToPatched(proto[LISTENERS_EVENT_LISTENER], nativeListeners);
    }
    return true;
  }
  let results = [];
  for (let i = 0; i < apis.length; i++) {
    results[i] = patchEventTargetMethods(apis[i], patchOptions);
  }
  return results;
}
function findEventTasks(target, eventName) {
  if (!eventName) {
    const foundTasks = [];
    for (let prop in target) {
      const match = EVENT_NAME_SYMBOL_REGX.exec(prop);
      let evtName = match && match[1];
      if (evtName && (!eventName || evtName === eventName)) {
        const tasks = target[prop];
        if (tasks) {
          for (let i = 0; i < tasks.length; i++) {
            foundTasks.push(tasks[i]);
          }
        }
      }
    }
    return foundTasks;
  }
  let symbolEventName = zoneSymbolEventNames2[eventName];
  if (!symbolEventName) {
    prepareEventNames(eventName);
    symbolEventName = zoneSymbolEventNames2[eventName];
  }
  const captureFalseTasks = target[symbolEventName[FALSE_STR]];
  const captureTrueTasks = target[symbolEventName[TRUE_STR]];
  if (!captureFalseTasks) {
    return captureTrueTasks ? captureTrueTasks.slice() : [];
  } else {
    return captureTrueTasks ? captureFalseTasks.concat(captureTrueTasks) : captureFalseTasks.slice();
  }
}
function patchEventPrototype(global2, api) {
  const Event = global2["Event"];
  if (Event && Event.prototype) {
    api.patchMethod(Event.prototype, "stopImmediatePropagation", (delegate) => function(self2, args) {
      self2[IMMEDIATE_PROPAGATION_SYMBOL] = true;
      delegate && delegate.apply(self2, args);
    });
  }
}

// packages/zone.js/lib/common/queue-microtask.js
function patchQueueMicrotask(global2, api) {
  api.patchMethod(global2, "queueMicrotask", (delegate) => {
    return function(self2, args) {
      Zone.current.scheduleMicroTask("queueMicrotask", args[0]);
    };
  });
}

// packages/zone.js/lib/common/timers.js
var taskSymbol = zoneSymbol("zoneTask");
function patchTimer(window2, setName, cancelName, nameSuffix) {
  let setNative = null;
  let clearNative = null;
  setName += nameSuffix;
  cancelName += nameSuffix;
  const tasksByHandleId = {};
  function scheduleTask(task) {
    const data = task.data;
    data.args[0] = function() {
      return task.invoke.apply(this, arguments);
    };
    const handleOrId = setNative.apply(window2, data.args);
    if (isNumber(handleOrId)) {
      data.handleId = handleOrId;
    } else {
      data.handle = handleOrId;
      data.isRefreshable = isFunction(handleOrId == null ? void 0 : handleOrId.refresh);
    }
    return task;
  }
  function clearTask(task) {
    const { handle, handleId } = task.data;
    return clearNative.call(window2, handle != null ? handle : handleId);
  }
  setNative = patchMethod(window2, setName, (delegate) => function(self2, args) {
    var _a;
    if (isFunction(args[0])) {
      const options = {
        isRefreshable: false,
        isPeriodic: nameSuffix === "Interval",
        delay: nameSuffix === "Timeout" || nameSuffix === "Interval" ? args[1] || 0 : void 0,
        args
      };
      const callback = args[0];
      args[0] = function timer() {
        try {
          return callback.apply(this, arguments);
        } finally {
          const { handle: handle2, handleId: handleId2, isPeriodic: isPeriodic2, isRefreshable: isRefreshable2 } = options;
          if (!isPeriodic2 && !isRefreshable2) {
            if (handleId2) {
              delete tasksByHandleId[handleId2];
            } else if (handle2) {
              handle2[taskSymbol] = null;
            }
          }
        }
      };
      const task = scheduleMacroTaskWithCurrentZone(setName, args[0], options, scheduleTask, clearTask);
      if (!task) {
        return task;
      }
      const { handleId, handle, isRefreshable, isPeriodic } = task.data;
      if (handleId) {
        tasksByHandleId[handleId] = task;
      } else if (handle) {
        handle[taskSymbol] = task;
        if (isRefreshable && !isPeriodic) {
          const originalRefresh = handle.refresh;
          handle.refresh = function() {
            const { zone, state } = task;
            if (state === "notScheduled") {
              task._state = "scheduled";
              zone._updateTaskCount(task, 1);
            } else if (state === "running") {
              task._state = "scheduling";
            }
            return originalRefresh.call(this);
          };
        }
      }
      return (_a = handle != null ? handle : handleId) != null ? _a : task;
    } else {
      return delegate.apply(window2, args);
    }
  });
  clearNative = patchMethod(window2, cancelName, (delegate) => function(self2, args) {
    const id = args[0];
    let task;
    if (isNumber(id)) {
      task = tasksByHandleId[id];
      delete tasksByHandleId[id];
    } else {
      task = id == null ? void 0 : id[taskSymbol];
      if (task) {
        id[taskSymbol] = null;
      } else {
        task = id;
      }
    }
    if (task == null ? void 0 : task.type) {
      if (task.cancelFn) {
        task.zone.cancelTask(task);
      }
    } else {
      delegate.apply(window2, args);
    }
  });
}

// packages/zone.js/lib/browser/custom-elements.js
function patchCustomElements(_global2, api) {
  const { isBrowser: isBrowser2, isMix: isMix2 } = api.getGlobalObjects();
  if (!isBrowser2 && !isMix2 || !_global2["customElements"] || !("customElements" in _global2)) {
    return;
  }
  const callbacks = [
    "connectedCallback",
    "disconnectedCallback",
    "adoptedCallback",
    "attributeChangedCallback",
    "formAssociatedCallback",
    "formDisabledCallback",
    "formResetCallback",
    "formStateRestoreCallback"
  ];
  api.patchCallbacks(api, _global2.customElements, "customElements", "define", callbacks);
}

// packages/zone.js/lib/browser/event-target.js
function eventTargetPatch(_global2, api) {
  if (Zone[api.symbol("patchEventTarget")]) {
    return;
  }
  const { eventNames, zoneSymbolEventNames: zoneSymbolEventNames3, TRUE_STR: TRUE_STR2, FALSE_STR: FALSE_STR2, ZONE_SYMBOL_PREFIX: ZONE_SYMBOL_PREFIX2 } = api.getGlobalObjects();
  for (let i = 0; i < eventNames.length; i++) {
    const eventName = eventNames[i];
    const falseEventName = eventName + FALSE_STR2;
    const trueEventName = eventName + TRUE_STR2;
    const symbol = ZONE_SYMBOL_PREFIX2 + falseEventName;
    const symbolCapture = ZONE_SYMBOL_PREFIX2 + trueEventName;
    zoneSymbolEventNames3[eventName] = {};
    zoneSymbolEventNames3[eventName][FALSE_STR2] = symbol;
    zoneSymbolEventNames3[eventName][TRUE_STR2] = symbolCapture;
  }
  const EVENT_TARGET = _global2["EventTarget"];
  if (!EVENT_TARGET || !EVENT_TARGET.prototype) {
    return;
  }
  api.patchEventTarget(_global2, api, [EVENT_TARGET && EVENT_TARGET.prototype]);
  return true;
}
function patchEvent(global2, api) {
  api.patchEventPrototype(global2, api);
}

// packages/zone.js/lib/browser/property-descriptor.js
function filterProperties(target, onProperties, ignoreProperties) {
  if (!ignoreProperties || ignoreProperties.length === 0) {
    return onProperties;
  }
  const tip = ignoreProperties.filter((ip) => ip.target === target);
  if (tip.length === 0) {
    return onProperties;
  }
  const targetIgnoreProperties = tip[0].ignoreProperties;
  return onProperties.filter((op) => targetIgnoreProperties.indexOf(op) === -1);
}
function patchFilteredProperties(target, onProperties, ignoreProperties, prototype) {
  if (!target) {
    return;
  }
  const filteredProperties = filterProperties(target, onProperties, ignoreProperties);
  patchOnProperties(target, filteredProperties, prototype);
}
function getOnEventNames(target) {
  return Object.getOwnPropertyNames(target).filter((name) => name.startsWith("on") && name.length > 2).map((name) => name.substring(2));
}
function propertyDescriptorPatch(api, _global2) {
  if (isNode && !isMix) {
    return;
  }
  if (Zone[api.symbol("patchEvents")]) {
    return;
  }
  const ignoreProperties = _global2["__Zone_ignore_on_properties"];
  let patchTargets = [];
  if (isBrowser) {
    const internalWindow2 = window;
    patchTargets = patchTargets.concat([
      "Document",
      "SVGElement",
      "Element",
      "HTMLElement",
      "HTMLBodyElement",
      "HTMLMediaElement",
      "HTMLFrameSetElement",
      "HTMLFrameElement",
      "HTMLIFrameElement",
      "HTMLMarqueeElement",
      "Worker"
    ]);
    patchFilteredProperties(internalWindow2, getOnEventNames(internalWindow2), ignoreProperties, ObjectGetPrototypeOf(internalWindow2));
  }
  patchTargets = patchTargets.concat([
    "XMLHttpRequest",
    "XMLHttpRequestEventTarget",
    "IDBIndex",
    "IDBRequest",
    "IDBOpenDBRequest",
    "IDBDatabase",
    "IDBTransaction",
    "IDBCursor",
    "WebSocket"
  ]);
  for (let i = 0; i < patchTargets.length; i++) {
    const target = _global2[patchTargets[i]];
    (target == null ? void 0 : target.prototype) && patchFilteredProperties(target.prototype, getOnEventNames(target.prototype), ignoreProperties);
  }
}

// packages/zone.js/lib/browser/browser.js
function patchBrowser(Zone3) {
  Zone3.__load_patch("timers", (global2) => {
    const set = "set";
    const clear = "clear";
    patchTimer(global2, set, clear, "Timeout");
    patchTimer(global2, set, clear, "Interval");
    patchTimer(global2, set, clear, "Immediate");
  });
  Zone3.__load_patch("requestAnimationFrame", (global2) => {
    patchTimer(global2, "request", "cancel", "AnimationFrame");
    patchTimer(global2, "mozRequest", "mozCancel", "AnimationFrame");
    patchTimer(global2, "webkitRequest", "webkitCancel", "AnimationFrame");
  });
  Zone3.__load_patch("blocking", (global2, Zone4) => {
    const blockingMethods = ["alert", "prompt", "confirm"];
    for (let i = 0; i < blockingMethods.length; i++) {
      const name = blockingMethods[i];
      patchMethod(global2, name, (delegate, symbol, name2) => {
        return function(s, args) {
          return Zone4.current.run(delegate, global2, args, name2);
        };
      });
    }
  });
  Zone3.__load_patch("EventTarget", (global2, Zone4, api) => {
    patchEvent(global2, api);
    eventTargetPatch(global2, api);
    const XMLHttpRequestEventTarget = global2["XMLHttpRequestEventTarget"];
    if (XMLHttpRequestEventTarget && XMLHttpRequestEventTarget.prototype) {
      api.patchEventTarget(global2, api, [XMLHttpRequestEventTarget.prototype]);
    }
  });
  Zone3.__load_patch("MutationObserver", (global2, Zone4, api) => {
    patchClass("MutationObserver");
    patchClass("WebKitMutationObserver");
  });
  Zone3.__load_patch("IntersectionObserver", (global2, Zone4, api) => {
    patchClass("IntersectionObserver");
  });
  Zone3.__load_patch("FileReader", (global2, Zone4, api) => {
    patchClass("FileReader");
  });
  Zone3.__load_patch("on_property", (global2, Zone4, api) => {
    propertyDescriptorPatch(api, global2);
  });
  Zone3.__load_patch("customElements", (global2, Zone4, api) => {
    patchCustomElements(global2, api);
  });
  Zone3.__load_patch("XHR", (global2, Zone4) => {
    patchXHR(global2);
    const XHR_TASK = zoneSymbol("xhrTask");
    const XHR_SYNC = zoneSymbol("xhrSync");
    const XHR_LISTENER = zoneSymbol("xhrListener");
    const XHR_SCHEDULED = zoneSymbol("xhrScheduled");
    const XHR_URL = zoneSymbol("xhrURL");
    const XHR_ERROR_BEFORE_SCHEDULED = zoneSymbol("xhrErrorBeforeScheduled");
    function patchXHR(window2) {
      const XMLHttpRequest = window2["XMLHttpRequest"];
      if (!XMLHttpRequest) {
        return;
      }
      const XMLHttpRequestPrototype = XMLHttpRequest.prototype;
      function findPendingTask(target) {
        return target[XHR_TASK];
      }
      let oriAddListener = XMLHttpRequestPrototype[ZONE_SYMBOL_ADD_EVENT_LISTENER];
      let oriRemoveListener = XMLHttpRequestPrototype[ZONE_SYMBOL_REMOVE_EVENT_LISTENER];
      if (!oriAddListener) {
        const XMLHttpRequestEventTarget = window2["XMLHttpRequestEventTarget"];
        if (XMLHttpRequestEventTarget) {
          const XMLHttpRequestEventTargetPrototype = XMLHttpRequestEventTarget.prototype;
          oriAddListener = XMLHttpRequestEventTargetPrototype[ZONE_SYMBOL_ADD_EVENT_LISTENER];
          oriRemoveListener = XMLHttpRequestEventTargetPrototype[ZONE_SYMBOL_REMOVE_EVENT_LISTENER];
        }
      }
      const READY_STATE_CHANGE = "readystatechange";
      const SCHEDULED = "scheduled";
      function scheduleTask(task) {
        const data = task.data;
        const target = data.target;
        target[XHR_SCHEDULED] = false;
        target[XHR_ERROR_BEFORE_SCHEDULED] = false;
        const listener = target[XHR_LISTENER];
        if (!oriAddListener) {
          oriAddListener = target[ZONE_SYMBOL_ADD_EVENT_LISTENER];
          oriRemoveListener = target[ZONE_SYMBOL_REMOVE_EVENT_LISTENER];
        }
        if (listener) {
          oriRemoveListener.call(target, READY_STATE_CHANGE, listener);
        }
        const newListener = target[XHR_LISTENER] = () => {
          if (target.readyState === target.DONE) {
            if (!data.aborted && target[XHR_SCHEDULED] && task.state === SCHEDULED) {
              const loadTasks = target[Zone4.__symbol__("loadfalse")];
              if (target.status !== 0 && loadTasks && loadTasks.length > 0) {
                const oriInvoke = task.invoke;
                task.invoke = function() {
                  const loadTasks2 = target[Zone4.__symbol__("loadfalse")];
                  for (let i = 0; i < loadTasks2.length; i++) {
                    if (loadTasks2[i] === task) {
                      loadTasks2.splice(i, 1);
                    }
                  }
                  if (!data.aborted && task.state === SCHEDULED) {
                    oriInvoke.call(task);
                  }
                };
                loadTasks.push(task);
              } else {
                task.invoke();
              }
            } else if (!data.aborted && target[XHR_SCHEDULED] === false) {
              target[XHR_ERROR_BEFORE_SCHEDULED] = true;
            }
          }
        };
        oriAddListener.call(target, READY_STATE_CHANGE, newListener);
        const storedTask = target[XHR_TASK];
        if (!storedTask) {
          target[XHR_TASK] = task;
        }
        sendNative.apply(target, data.args);
        target[XHR_SCHEDULED] = true;
        return task;
      }
      function placeholderCallback() {
      }
      function clearTask(task) {
        const data = task.data;
        data.aborted = true;
        return abortNative.apply(data.target, data.args);
      }
      const openNative = patchMethod(XMLHttpRequestPrototype, "open", () => function(self2, args) {
        self2[XHR_SYNC] = args[2] == false;
        self2[XHR_URL] = args[1];
        return openNative.apply(self2, args);
      });
      const XMLHTTPREQUEST_SOURCE = "XMLHttpRequest.send";
      const fetchTaskAborting = zoneSymbol("fetchTaskAborting");
      const fetchTaskScheduling = zoneSymbol("fetchTaskScheduling");
      const sendNative = patchMethod(XMLHttpRequestPrototype, "send", () => function(self2, args) {
        if (Zone4.current[fetchTaskScheduling] === true) {
          return sendNative.apply(self2, args);
        }
        if (self2[XHR_SYNC]) {
          return sendNative.apply(self2, args);
        } else {
          const options = {
            target: self2,
            url: self2[XHR_URL],
            isPeriodic: false,
            args,
            aborted: false
          };
          const task = scheduleMacroTaskWithCurrentZone(XMLHTTPREQUEST_SOURCE, placeholderCallback, options, scheduleTask, clearTask);
          if (self2 && self2[XHR_ERROR_BEFORE_SCHEDULED] === true && !options.aborted && task.state === SCHEDULED) {
            task.invoke();
          }
        }
      });
      const abortNative = patchMethod(XMLHttpRequestPrototype, "abort", () => function(self2, args) {
        const task = findPendingTask(self2);
        if (task && typeof task.type == "string") {
          if (task.cancelFn == null || task.data && task.data.aborted) {
            return;
          }
          task.zone.cancelTask(task);
        } else if (Zone4.current[fetchTaskAborting] === true) {
          return abortNative.apply(self2, args);
        }
      });
    }
  });
  Zone3.__load_patch("geolocation", (global2) => {
    if (global2["navigator"] && global2["navigator"].geolocation) {
      patchPrototype(global2["navigator"].geolocation, ["getCurrentPosition", "watchPosition"]);
    }
  });
  Zone3.__load_patch("PromiseRejectionEvent", (global2, Zone4) => {
    function findPromiseRejectionHandler(evtName) {
      return function(e) {
        const eventTasks = findEventTasks(global2, evtName);
        eventTasks.forEach((eventTask) => {
          const PromiseRejectionEvent = global2["PromiseRejectionEvent"];
          if (PromiseRejectionEvent) {
            const evt = new PromiseRejectionEvent(evtName, {
              promise: e.promise,
              reason: e.rejection
            });
            eventTask.invoke(evt);
          }
        });
      };
    }
    if (global2["PromiseRejectionEvent"]) {
      Zone4[zoneSymbol("unhandledPromiseRejectionHandler")] = findPromiseRejectionHandler("unhandledrejection");
      Zone4[zoneSymbol("rejectionHandledHandler")] = findPromiseRejectionHandler("rejectionhandled");
    }
  });
  Zone3.__load_patch("queueMicrotask", (global2, Zone4, api) => {
    patchQueueMicrotask(global2, api);
  });
}

// packages/zone.js/lib/common/promise.js
function patchPromise(Zone3) {
  Zone3.__load_patch("ZoneAwarePromise", (global2, Zone4, api) => {
    const ObjectGetOwnPropertyDescriptor2 = Object.getOwnPropertyDescriptor;
    const ObjectDefineProperty2 = Object.defineProperty;
    function readableObjectToString(obj) {
      if (obj && obj.toString === Object.prototype.toString) {
        const className = obj.constructor && obj.constructor.name;
        return (className ? className : "") + ": " + JSON.stringify(obj);
      }
      return obj ? obj.toString() : Object.prototype.toString.call(obj);
    }
    const __symbol__2 = api.symbol;
    const _uncaughtPromiseErrors = [];
    const isDisableWrappingUncaughtPromiseRejection = global2[__symbol__2("DISABLE_WRAPPING_UNCAUGHT_PROMISE_REJECTION")] !== false;
    const symbolPromise = __symbol__2("Promise");
    const symbolThen = __symbol__2("then");
    const creationTrace = "__creationTrace__";
    api.onUnhandledError = (e) => {
      if (api.showUncaughtError()) {
        const rejection = e && e.rejection;
        if (rejection && e.zone && e.task) {
          console.error("Unhandled Promise rejection:", rejection instanceof Error ? rejection.message : rejection, "; Zone:", e.zone.name, "; Task:", e.task && e.task.source, "; Value:", rejection, rejection instanceof Error ? rejection.stack : void 0);
        } else {
          console.error(e);
        }
      }
    };
    api.microtaskDrainDone = () => {
      while (_uncaughtPromiseErrors.length) {
        const uncaughtPromiseError = _uncaughtPromiseErrors.shift();
        try {
          uncaughtPromiseError.zone.runGuarded(() => {
            if (uncaughtPromiseError.throwOriginal) {
              throw uncaughtPromiseError.rejection;
            }
            throw uncaughtPromiseError;
          });
        } catch (error) {
          handleUnhandledRejection(error);
        }
      }
    };
    const UNHANDLED_PROMISE_REJECTION_HANDLER_SYMBOL = __symbol__2("unhandledPromiseRejectionHandler");
    function handleUnhandledRejection(e) {
      api.onUnhandledError(e);
      try {
        const handler = Zone4[UNHANDLED_PROMISE_REJECTION_HANDLER_SYMBOL];
        if (typeof handler === "function") {
          handler.call(this, e);
        }
      } catch (err) {
      }
    }
    function isThenable(value) {
      return value && typeof value.then === "function";
    }
    function forwardResolution(value) {
      return value;
    }
    function forwardRejection(rejection) {
      return ZoneAwarePromise.reject(rejection);
    }
    const symbolState = __symbol__2("state");
    const symbolValue = __symbol__2("value");
    const symbolFinally = __symbol__2("finally");
    const symbolParentPromiseValue = __symbol__2("parentPromiseValue");
    const symbolParentPromiseState = __symbol__2("parentPromiseState");
    const source = "Promise.then";
    const UNRESOLVED = null;
    const RESOLVED = true;
    const REJECTED = false;
    const REJECTED_NO_CATCH = 0;
    function makeResolver(promise, state) {
      return (v) => {
        try {
          resolvePromise(promise, state, v);
        } catch (err) {
          resolvePromise(promise, false, err);
        }
      };
    }
    const once = function() {
      let wasCalled = false;
      return function wrapper(wrappedFunction) {
        return function() {
          if (wasCalled) {
            return;
          }
          wasCalled = true;
          wrappedFunction.apply(null, arguments);
        };
      };
    };
    const TYPE_ERROR = "Promise resolved with itself";
    const CURRENT_TASK_TRACE_SYMBOL = __symbol__2("currentTaskTrace");
    function resolvePromise(promise, state, value) {
      const onceWrapper = once();
      if (promise === value) {
        throw new TypeError(TYPE_ERROR);
      }
      if (promise[symbolState] === UNRESOLVED) {
        let then = null;
        try {
          if (typeof value === "object" || typeof value === "function") {
            then = value && value.then;
          }
        } catch (err) {
          onceWrapper(() => {
            resolvePromise(promise, false, err);
          })();
          return promise;
        }
        if (state !== REJECTED && value instanceof ZoneAwarePromise && Object.hasOwn(value, symbolState) && Object.hasOwn(value, symbolValue) && value[symbolState] !== UNRESOLVED) {
          clearRejectedNoCatch(value);
          resolvePromise(promise, value[symbolState], value[symbolValue]);
        } else if (state !== REJECTED && typeof then === "function") {
          try {
            then.call(value, onceWrapper(makeResolver(promise, state)), onceWrapper(makeResolver(promise, false)));
          } catch (err) {
            onceWrapper(() => {
              resolvePromise(promise, false, err);
            })();
          }
        } else {
          promise[symbolState] = state;
          const queue = promise[symbolValue];
          promise[symbolValue] = value;
          if (promise[symbolFinally] === symbolFinally) {
            if (state === RESOLVED) {
              promise[symbolState] = promise[symbolParentPromiseState];
              promise[symbolValue] = promise[symbolParentPromiseValue];
            }
          }
          if (state === REJECTED && value instanceof Error) {
            const trace = Zone4.currentTask && Zone4.currentTask.data && Zone4.currentTask.data[creationTrace];
            if (trace) {
              ObjectDefineProperty2(value, CURRENT_TASK_TRACE_SYMBOL, {
                configurable: true,
                enumerable: false,
                writable: true,
                value: trace
              });
            }
          }
          for (let i = 0; i < queue.length; ) {
            scheduleResolveOrReject(promise, queue[i++], queue[i++], queue[i++], queue[i++]);
          }
          if (queue.length == 0 && state == REJECTED) {
            promise[symbolState] = REJECTED_NO_CATCH;
            let uncaughtPromiseError = value;
            try {
              throw new Error("Uncaught (in promise): " + readableObjectToString(value) + (value && value.stack ? "\n" + value.stack : ""));
            } catch (err) {
              uncaughtPromiseError = err;
            }
            if (isDisableWrappingUncaughtPromiseRejection) {
              uncaughtPromiseError.throwOriginal = true;
            }
            uncaughtPromiseError.rejection = value;
            uncaughtPromiseError.promise = promise;
            uncaughtPromiseError.zone = Zone4.current;
            uncaughtPromiseError.task = Zone4.currentTask;
            _uncaughtPromiseErrors.push(uncaughtPromiseError);
            api.scheduleMicroTask();
          }
        }
      }
      return promise;
    }
    const REJECTION_HANDLED_HANDLER = __symbol__2("rejectionHandledHandler");
    function clearRejectedNoCatch(promise) {
      if (promise[symbolState] === REJECTED_NO_CATCH) {
        try {
          const handler = Zone4[REJECTION_HANDLED_HANDLER];
          if (handler && typeof handler === "function") {
            handler.call(this, { rejection: promise[symbolValue], promise });
          }
        } catch (err) {
        }
        promise[symbolState] = REJECTED;
        for (let i = 0; i < _uncaughtPromiseErrors.length; i++) {
          if (promise === _uncaughtPromiseErrors[i].promise) {
            _uncaughtPromiseErrors.splice(i, 1);
          }
        }
      }
    }
    function scheduleResolveOrReject(promise, zone, chainPromise, onFulfilled, onRejected) {
      clearRejectedNoCatch(promise);
      const promiseState = promise[symbolState];
      const delegate = promiseState ? typeof onFulfilled === "function" ? onFulfilled : forwardResolution : typeof onRejected === "function" ? onRejected : forwardRejection;
      zone.scheduleMicroTask(source, () => {
        try {
          const parentPromiseValue = promise[symbolValue];
          const isFinallyPromise = !!chainPromise && symbolFinally === chainPromise[symbolFinally];
          if (isFinallyPromise) {
            chainPromise[symbolParentPromiseValue] = parentPromiseValue;
            chainPromise[symbolParentPromiseState] = promiseState;
          }
          const value = zone.run(delegate, void 0, isFinallyPromise && delegate !== forwardRejection && delegate !== forwardResolution ? [] : [parentPromiseValue]);
          resolvePromise(chainPromise, true, value);
        } catch (error) {
          resolvePromise(chainPromise, false, error);
        }
      }, chainPromise);
    }
    const ZONE_AWARE_PROMISE_TO_STRING = "function ZoneAwarePromise() { [native code] }";
    const noop = function() {
    };
    const AggregateError = global2.AggregateError;
    class ZoneAwarePromise {
      static toString() {
        return ZONE_AWARE_PROMISE_TO_STRING;
      }
      static resolve(value) {
        if (value instanceof ZoneAwarePromise) {
          return value;
        }
        return resolvePromise(new this(null), RESOLVED, value);
      }
      static reject(error) {
        return resolvePromise(new this(null), REJECTED, error);
      }
      static withResolvers() {
        const result = {};
        result.promise = new ZoneAwarePromise((res, rej) => {
          result.resolve = res;
          result.reject = rej;
        });
        return result;
      }
      static any(values) {
        if (!values || typeof values[Symbol.iterator] !== "function") {
          return Promise.reject(new AggregateError([], "All promises were rejected"));
        }
        const promises = [];
        let count = 0;
        try {
          for (let v of values) {
            count++;
            promises.push(ZoneAwarePromise.resolve(v));
          }
        } catch (err) {
          return Promise.reject(new AggregateError([], "All promises were rejected"));
        }
        if (count === 0) {
          return Promise.reject(new AggregateError([], "All promises were rejected"));
        }
        let finished = false;
        const errors = [];
        return new ZoneAwarePromise((resolve, reject) => {
          for (let i = 0; i < promises.length; i++) {
            promises[i].then((v) => {
              if (finished) {
                return;
              }
              finished = true;
              resolve(v);
            }, (err) => {
              errors.push(err);
              count--;
              if (count === 0) {
                finished = true;
                reject(new AggregateError(errors, "All promises were rejected"));
              }
            });
          }
        });
      }
      static race(values) {
        let resolve;
        let reject;
        let promise = new this((res, rej) => {
          resolve = res;
          reject = rej;
        });
        function onResolve(value) {
          resolve(value);
        }
        function onReject(error) {
          reject(error);
        }
        for (let value of values) {
          if (!isThenable(value)) {
            value = this.resolve(value);
          }
          value.then(onResolve, onReject);
        }
        return promise;
      }
      static all(values) {
        return ZoneAwarePromise.allWithCallback(values);
      }
      static allSettled(values) {
        const P = this && this.prototype instanceof ZoneAwarePromise ? this : ZoneAwarePromise;
        return P.allWithCallback(values, {
          thenCallback: (value) => ({ status: "fulfilled", value }),
          errorCallback: (err) => ({ status: "rejected", reason: err })
        });
      }
      static allWithCallback(values, callback) {
        let resolve;
        let reject;
        let promise = new this((res, rej) => {
          resolve = res;
          reject = rej;
        });
        let unresolvedCount = 2;
        let valueIndex = 0;
        const resolvedValues = [];
        for (let value of values) {
          if (!isThenable(value)) {
            value = this.resolve(value);
          }
          const curValueIndex = valueIndex;
          try {
            value.then((value2) => {
              resolvedValues[curValueIndex] = callback ? callback.thenCallback(value2) : value2;
              unresolvedCount--;
              if (unresolvedCount === 0) {
                resolve(resolvedValues);
              }
            }, (err) => {
              if (!callback) {
                reject(err);
              } else {
                resolvedValues[curValueIndex] = callback.errorCallback(err);
                unresolvedCount--;
                if (unresolvedCount === 0) {
                  resolve(resolvedValues);
                }
              }
            });
          } catch (thenErr) {
            reject(thenErr);
          }
          unresolvedCount++;
          valueIndex++;
        }
        unresolvedCount -= 2;
        if (unresolvedCount === 0) {
          resolve(resolvedValues);
        }
        return promise;
      }
      constructor(executor) {
        const promise = this;
        if (!(promise instanceof ZoneAwarePromise)) {
          throw new Error("Must be an instanceof Promise.");
        }
        promise[symbolState] = UNRESOLVED;
        promise[symbolValue] = [];
        try {
          const onceWrapper = once();
          executor && executor(onceWrapper(makeResolver(promise, RESOLVED)), onceWrapper(makeResolver(promise, REJECTED)));
        } catch (error) {
          resolvePromise(promise, false, error);
        }
      }
      get [Symbol.toStringTag]() {
        return "Promise";
      }
      get [Symbol.species]() {
        return ZoneAwarePromise;
      }
      then(onFulfilled, onRejected) {
        var _a;
        let C = (_a = this.constructor) == null ? void 0 : _a[Symbol.species];
        if (!C || typeof C !== "function") {
          C = this.constructor || ZoneAwarePromise;
        }
        const chainPromise = new C(noop);
        const zone = Zone4.current;
        if (this[symbolState] == UNRESOLVED) {
          this[symbolValue].push(zone, chainPromise, onFulfilled, onRejected);
        } else {
          scheduleResolveOrReject(this, zone, chainPromise, onFulfilled, onRejected);
        }
        return chainPromise;
      }
      catch(onRejected) {
        return this.then(null, onRejected);
      }
      finally(onFinally) {
        var _a;
        let C = (_a = this.constructor) == null ? void 0 : _a[Symbol.species];
        if (!C || typeof C !== "function") {
          C = ZoneAwarePromise;
        }
        const chainPromise = new C(noop);
        chainPromise[symbolFinally] = symbolFinally;
        const zone = Zone4.current;
        if (this[symbolState] == UNRESOLVED) {
          this[symbolValue].push(zone, chainPromise, onFinally, onFinally);
        } else {
          scheduleResolveOrReject(this, zone, chainPromise, onFinally, onFinally);
        }
        return chainPromise;
      }
    }
    ZoneAwarePromise["resolve"] = ZoneAwarePromise.resolve;
    ZoneAwarePromise["reject"] = ZoneAwarePromise.reject;
    ZoneAwarePromise["race"] = ZoneAwarePromise.race;
    ZoneAwarePromise["all"] = ZoneAwarePromise.all;
    const NativePromise = global2[symbolPromise] = global2["Promise"];
    global2["Promise"] = ZoneAwarePromise;
    const symbolThenPatched = __symbol__2("thenPatched");
    function patchThen(Ctor) {
      const proto = Ctor.prototype;
      const prop = ObjectGetOwnPropertyDescriptor2(proto, "then");
      if (prop && (prop.writable === false || !prop.configurable)) {
        return;
      }
      const originalThen = proto.then;
      proto[symbolThen] = originalThen;
      Ctor.prototype.then = function(onResolve, onReject) {
        const wrapped = new ZoneAwarePromise((resolve, reject) => {
          originalThen.call(this, resolve, reject);
        });
        return wrapped.then(onResolve, onReject);
      };
      Ctor[symbolThenPatched] = true;
    }
    api.patchThen = patchThen;
    function zoneify(fn) {
      return function(self2, args) {
        let resultPromise = fn.apply(self2, args);
        if (resultPromise instanceof ZoneAwarePromise) {
          return resultPromise;
        }
        let ctor = resultPromise.constructor;
        if (!ctor[symbolThenPatched]) {
          patchThen(ctor);
        }
        return resultPromise;
      };
    }
    if (NativePromise) {
      patchThen(NativePromise);
      const nativeTry = NativePromise["try"];
      if (nativeTry && typeof nativeTry === "function") {
        ZoneAwarePromise["try"] = nativeTry;
      }
      patchMethod(global2, "fetch", (delegate) => zoneify(delegate));
    }
    Promise[Zone4.__symbol__("uncaughtPromiseErrors")] = _uncaughtPromiseErrors;
    return ZoneAwarePromise;
  });
}

// packages/zone.js/lib/common/to-string.js
function patchToString(Zone3) {
  Zone3.__load_patch("toString", (global2) => {
    const originalFunctionToString = Function.prototype.toString;
    const ORIGINAL_DELEGATE_SYMBOL = zoneSymbol("OriginalDelegate");
    const PROMISE_SYMBOL = zoneSymbol("Promise");
    const ERROR_SYMBOL = zoneSymbol("Error");
    const newFunctionToString = function toString() {
      if (typeof this === "function") {
        const originalDelegate = this[ORIGINAL_DELEGATE_SYMBOL];
        if (originalDelegate) {
          if (typeof originalDelegate === "function") {
            return originalFunctionToString.call(originalDelegate);
          } else {
            return Object.prototype.toString.call(originalDelegate);
          }
        }
        if (this === Promise) {
          const nativePromise = global2[PROMISE_SYMBOL];
          if (nativePromise) {
            return originalFunctionToString.call(nativePromise);
          }
        }
        if (this === Error) {
          const nativeError = global2[ERROR_SYMBOL];
          if (nativeError) {
            return originalFunctionToString.call(nativeError);
          }
        }
      }
      return originalFunctionToString.call(this);
    };
    newFunctionToString[ORIGINAL_DELEGATE_SYMBOL] = originalFunctionToString;
    Function.prototype.toString = newFunctionToString;
    const originalObjectToString = Object.prototype.toString;
    const PROMISE_OBJECT_TO_STRING = "[object Promise]";
    Object.prototype.toString = function() {
      if (typeof Promise === "function" && this instanceof Promise) {
        return PROMISE_OBJECT_TO_STRING;
      }
      return originalObjectToString.call(this);
    };
  });
}

// packages/zone.js/lib/browser/browser-util.js
function patchCallbacks(api, target, targetName, method, callbacks) {
  const symbol = Zone.__symbol__(method);
  if (target[symbol]) {
    return;
  }
  const nativeDelegate = target[symbol] = target[method];
  target[method] = function(name, opts, options) {
    if (opts && opts.prototype) {
      callbacks.forEach(function(callback) {
        const source = `${targetName}.${method}::` + callback;
        const prototype = opts.prototype;
        try {
          if (Object.hasOwn(prototype, callback)) {
            const descriptor = api.ObjectGetOwnPropertyDescriptor(prototype, callback);
            if (descriptor && descriptor.value) {
              descriptor.value = api.wrapWithCurrentZone(descriptor.value, source);
              api._redefineProperty(opts.prototype, callback, descriptor);
            } else if (prototype[callback]) {
              prototype[callback] = api.wrapWithCurrentZone(prototype[callback], source);
            }
          } else if (prototype[callback]) {
            prototype[callback] = api.wrapWithCurrentZone(prototype[callback], source);
          }
        } catch (e) {
        }
      });
    }
    return nativeDelegate.call(target, name, opts, options);
  };
  api.attachOriginToPatched(target[method], nativeDelegate);
}

// packages/zone.js/lib/browser/api-util.js
function patchUtil(Zone3) {
  Zone3.__load_patch("util", (global2, Zone4, api) => {
    const eventNames = getOnEventNames(global2);
    api.patchOnProperties = patchOnProperties;
    api.patchMethod = patchMethod;
    api.bindArguments = bindArguments;
    api.patchMacroTask = patchMacroTask;
    const SYMBOL_BLACK_LISTED_EVENTS = Zone4.__symbol__("BLACK_LISTED_EVENTS");
    const SYMBOL_UNPATCHED_EVENTS = Zone4.__symbol__("UNPATCHED_EVENTS");
    if (global2[SYMBOL_UNPATCHED_EVENTS]) {
      global2[SYMBOL_BLACK_LISTED_EVENTS] = global2[SYMBOL_UNPATCHED_EVENTS];
    }
    if (global2[SYMBOL_BLACK_LISTED_EVENTS]) {
      Zone4[SYMBOL_BLACK_LISTED_EVENTS] = Zone4[SYMBOL_UNPATCHED_EVENTS] = global2[SYMBOL_BLACK_LISTED_EVENTS];
    }
    api.patchEventPrototype = patchEventPrototype;
    api.patchEventTarget = patchEventTarget;
    api.ObjectDefineProperty = ObjectDefineProperty;
    api.ObjectGetOwnPropertyDescriptor = ObjectGetOwnPropertyDescriptor;
    api.ObjectCreate = ObjectCreate;
    api.ArraySlice = ArraySlice;
    api.patchClass = patchClass;
    api.wrapWithCurrentZone = wrapWithCurrentZone;
    api.filterProperties = filterProperties;
    api.attachOriginToPatched = attachOriginToPatched;
    api._redefineProperty = Object.defineProperty;
    api.patchCallbacks = patchCallbacks;
    api.getGlobalObjects = () => ({
      globalSources,
      zoneSymbolEventNames: zoneSymbolEventNames2,
      eventNames,
      isBrowser,
      isMix,
      isNode,
      TRUE_STR,
      FALSE_STR,
      ZONE_SYMBOL_PREFIX,
      ADD_EVENT_LISTENER_STR,
      REMOVE_EVENT_LISTENER_STR
    });
  });
}

// packages/zone.js/lib/browser/rollup-common.js
function patchCommon(Zone3) {
  patchPromise(Zone3);
  patchToString(Zone3);
  patchUtil(Zone3);
}

// packages/zone.js/lib/browser/rollup-main.js
var Zone2 = loadZone();
patchCommon(Zone2);
patchBrowser(Zone2);


/***/ }),

/***/ 58221:
/***/ ((module) => {

/**
 * Helpers.
 */

var s = 1000;
var m = s * 60;
var h = m * 60;
var d = h * 24;
var w = d * 7;
var y = d * 365.25;

/**
 * Parse or format the given `val`.
 *
 * Options:
 *
 *  - `long` verbose formatting [false]
 *
 * @param {String|Number} val
 * @param {Object} [options]
 * @throws {Error} throw an error if val is not a non-empty string or a number
 * @return {String|Number}
 * @api public
 */

module.exports = function (val, options) {
  options = options || {};
  var type = typeof val;
  if (type === 'string' && val.length > 0) {
    return parse(val);
  } else if (type === 'number' && isFinite(val)) {
    return options.long ? fmtLong(val) : fmtShort(val);
  }
  throw new Error(
    'val is not a non-empty string or a valid number. val=' +
      JSON.stringify(val)
  );
};

/**
 * Parse the given `str` and return milliseconds.
 *
 * @param {String} str
 * @return {Number}
 * @api private
 */

function parse(str) {
  str = String(str);
  if (str.length > 100) {
    return;
  }
  var match = /^(-?(?:\d+)?\.?\d+) *(milliseconds?|msecs?|ms|seconds?|secs?|s|minutes?|mins?|m|hours?|hrs?|h|days?|d|weeks?|w|years?|yrs?|y)?$/i.exec(
    str
  );
  if (!match) {
    return;
  }
  var n = parseFloat(match[1]);
  var type = (match[2] || 'ms').toLowerCase();
  switch (type) {
    case 'years':
    case 'year':
    case 'yrs':
    case 'yr':
    case 'y':
      return n * y;
    case 'weeks':
    case 'week':
    case 'w':
      return n * w;
    case 'days':
    case 'day':
    case 'd':
      return n * d;
    case 'hours':
    case 'hour':
    case 'hrs':
    case 'hr':
    case 'h':
      return n * h;
    case 'minutes':
    case 'minute':
    case 'mins':
    case 'min':
    case 'm':
      return n * m;
    case 'seconds':
    case 'second':
    case 'secs':
    case 'sec':
    case 's':
      return n * s;
    case 'milliseconds':
    case 'millisecond':
    case 'msecs':
    case 'msec':
    case 'ms':
      return n;
    default:
      return undefined;
  }
}

/**
 * Short format for `ms`.
 *
 * @param {Number} ms
 * @return {String}
 * @api private
 */

function fmtShort(ms) {
  var msAbs = Math.abs(ms);
  if (msAbs >= d) {
    return Math.round(ms / d) + 'd';
  }
  if (msAbs >= h) {
    return Math.round(ms / h) + 'h';
  }
  if (msAbs >= m) {
    return Math.round(ms / m) + 'm';
  }
  if (msAbs >= s) {
    return Math.round(ms / s) + 's';
  }
  return ms + 'ms';
}

/**
 * Long format for `ms`.
 *
 * @param {Number} ms
 * @return {String}
 * @api private
 */

function fmtLong(ms) {
  var msAbs = Math.abs(ms);
  if (msAbs >= d) {
    return plural(ms, msAbs, d, 'day');
  }
  if (msAbs >= h) {
    return plural(ms, msAbs, h, 'hour');
  }
  if (msAbs >= m) {
    return plural(ms, msAbs, m, 'minute');
  }
  if (msAbs >= s) {
    return plural(ms, msAbs, s, 'second');
  }
  return ms + ' ms';
}

/**
 * Pluralization helper.
 */

function plural(ms, msAbs, n, name) {
  var isPlural = msAbs >= n * 1.5;
  return Math.round(ms / n) + ' ' + name + (isPlural ? 's' : '');
}


/***/ }),

/***/ 60113:
/***/ ((module) => {

"use strict";


module.exports = (flag, argv = process.argv) => {
	const prefix = flag.startsWith('-') ? '' : (flag.length === 1 ? '-' : '--');
	const position = argv.indexOf(prefix + flag);
	const terminatorPosition = argv.indexOf('--');
	return position !== -1 && (terminatorPosition === -1 || position < terminatorPosition);
};


/***/ }),

/***/ 64340:
/***/ ((module, __unused_webpack_exports, __webpack_require__) => {

// Unless explicitly stated otherwise all files in this repository are licensed under the Apache 2.0 License.
//
// This product includes software developed at Datadog (https://www.datadoghq.com/). Copyright 2021 Datadog, Inc.

const path = __webpack_require__(33873)
const moduleDetailsFromPath = __webpack_require__(70290)
const { fileURLToPath } = __webpack_require__(79551)
const { MessageChannel } = __webpack_require__(73566)

let { isBuiltin } = __webpack_require__(8086)
if (!isBuiltin) {
  isBuiltin = () => true
}

const {
  importHooks,
  specifiers,
  toHook
} = __webpack_require__(21539)

/**
 * Checks turbopack specifiers separately (for Next.js 16+).
 *
 * If turbopack is used, specifiers will have an additional hash appended to the end.
 * Something like "ai" might become "ai-5e7181a616786b24". This only happens in Next.js 16+.
 * Just checking if the baseDir ends with this new specifier won't match, as the baseDir still has the plain package.
 *
 * This logic isolates a new check for checking the actual name in the case turbopack is being used.
 *
 * @param specifier {string}
 * @param baseDir {string}
 */
function isTurbopackSpecifier (specifier, baseDir) {
  const usingTurbopack = false ?? 0
  if (!usingTurbopack) return false

  const specifierWithoutTurbopackHash = specifier.slice(0, specifier.lastIndexOf('-'))
  return baseDir.endsWith(specifierWithoutTurbopackHash)
}

function addHook (hook) {
  importHooks.push(hook)
  toHook.forEach(([name, namespace, specifier]) => hook(name, namespace, specifier))
}

function removeHook (hook) {
  const index = importHooks.indexOf(hook)
  if (index > -1) {
    importHooks.splice(index, 1)
  }
}

function callHookFn (hookFn, namespace, name, baseDir) {
  const newDefault = hookFn(namespace, name, baseDir)
  if (newDefault && newDefault !== namespace) {
    // Only ESM modules that actually export `default` can have it reassigned.
    // Some hooks return a value unconditionally; avoid crashing when the module
    // has no default export (see issue #188).
    if ('default' in namespace) {
      namespace.default = newDefault
    }
  }
}

let sendModulesToLoader

/**
 * EXPERIMENTAL
 * This feature is experimental and may change in minor versions.
 * **NOTE** This feature is incompatible with the {internals: true} Hook option.
 *
 * Creates a message channel with a port that can be used to add hooks to the
 * list of exclusively included modules.
 *
 * This can be used to only wrap modules that are Hook'ed, however modules need
 * to be hooked before they are imported.
 *
 * ```ts
 * import { register } from 'module'
 * import { Hook, createAddHookMessageChannel } from 'import-in-the-middle'
 *
 * const { registerOptions, waitForAllMessagesAcknowledged } = createAddHookMessageChannel()
 *
 * register('import-in-the-middle/hook.mjs', import.meta.url, registerOptions)
 *
 * Hook(['fs'], (exported, name, baseDir) => {
 *   // Instrument the fs module
 * })
 *
 * // Ensure that the loader has acknowledged all the modules
 * // before we allow execution to continue
 * await waitForAllMessagesAcknowledged()
 * ```
 */
function createAddHookMessageChannel () {
  const { port1, port2 } = new MessageChannel()
  let pendingAckCount = 0
  let resolveFn

  sendModulesToLoader = (modules) => {
    pendingAckCount++
    port1.postMessage(modules)
  }

  port1.on('message', () => {
    pendingAckCount--

    if (resolveFn && pendingAckCount <= 0) {
      resolveFn()
    }
  }).unref()

  function waitForAllMessagesAcknowledged () {
    // This timer is to prevent the process from exiting with code 13:
    // 13: Unsettled Top-Level Await.
    const timer = setInterval(() => { }, 1000)
    const promise = new Promise((resolve) => {
      resolveFn = resolve
    }).then(() => { clearInterval(timer) })

    if (pendingAckCount === 0) {
      resolveFn()
    }

    return promise
  }

  const addHookMessagePort = port2
  const registerOptions = { data: { addHookMessagePort, include: [] }, transferList: [addHookMessagePort] }

  return { registerOptions, addHookMessagePort, waitForAllMessagesAcknowledged }
}

function Hook (modules, options, hookFn) {
  if ((this instanceof Hook) === false) return new Hook(modules, options, hookFn)
  if (typeof modules === 'function') {
    hookFn = modules
    modules = null
    options = null
  } else if (typeof options === 'function') {
    hookFn = options
    options = null
  }
  const internals = options ? options.internals === true : false

  if (sendModulesToLoader && Array.isArray(modules)) {
    sendModulesToLoader(modules)
  }

  this._iitmHook = (name, namespace, specifier) => {
    const loadUrl = name
    const isNodeUrl = loadUrl.startsWith('node:')
    let filePath, baseDir

    if (isNodeUrl) {
      // Normalize builtin module name to *not* have 'node:' prefix, unless
      // required, as it is for 'node:test' and some others.  `module.isBuiltin`
      // is available in all Node.js versions that have node:-only modules.
      const unprefixed = name.slice(5)
      if (isBuiltin(unprefixed)) {
        name = unprefixed
      }
    } else if (loadUrl.startsWith('file://')) {
      const stackTraceLimit = Error.stackTraceLimit
      Error.stackTraceLimit = 0
      try {
        filePath = fileURLToPath(name)
        name = filePath
      } catch (e) {}
      Error.stackTraceLimit = stackTraceLimit

      if (filePath) {
        const details = moduleDetailsFromPath(filePath)
        if (details) {
          name = details.name
          baseDir = details.basedir
        }
      }
    }

    if (modules) {
      for (const matchArg of modules) {
        if (filePath && matchArg === filePath) {
          // abspath match
          callHookFn(hookFn, namespace, filePath, undefined)
        } else if (matchArg === name) {
          if (!baseDir) {
            // built-in module (or unexpected non file:// name?)
            callHookFn(hookFn, namespace, name, baseDir)
          } else if (baseDir.endsWith(specifiers.get(loadUrl)) || isTurbopackSpecifier(specifiers.get(loadUrl), baseDir)) {
            // An import of the top-level module (e.g. `import 'ioredis'`).
            // Note: Slight behaviour difference from RITM. RITM uses
            // `require.resolve(name)` to see if filename is the module
            // main file, which will catch `require('ioredis/built/index.js')`.
            // The check here will not catch `import 'ioredis/built/index.js'`.
            callHookFn(hookFn, namespace, name, baseDir)
          } else if (internals) {
            const internalPath = name + path.sep + path.relative(baseDir, filePath)
            callHookFn(hookFn, namespace, internalPath, baseDir)
          }
        } else if (matchArg === specifier) {
          callHookFn(hookFn, namespace, specifier, baseDir)
        }
      }
    } else {
      callHookFn(hookFn, namespace, name, baseDir)
    }
  }

  addHook(this._iitmHook)
}

Hook.prototype.unhook = function () {
  removeHook(this._iitmHook)
}

module.exports = Hook
module.exports.Hook = Hook
module.exports.addHook = addHook
module.exports.removeHook = removeHook
module.exports.createAddHookMessageChannel = createAddHookMessageChannel


/***/ }),

/***/ 69032:
/***/ ((__unused_webpack_module, __unused_webpack___webpack_exports__, __webpack_require__) => {

"use strict";

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api@1.9.1/node_modules/@opentelemetry/api/build/esm/trace-api.js + 6 modules
var trace_api = __webpack_require__(12577);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api@1.9.1/node_modules/@opentelemetry/api/build/esm/metrics/NoopMeter.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
/**
 * NoopMeter is a noop implementation of the {@link Meter} interface. It reuses
 * constant NoopMetrics for all of its methods.
 */
class NoopMeter {
    constructor() { }
    /**
     * @see {@link Meter.createGauge}
     */
    createGauge(_name, _options) {
        return NOOP_GAUGE_METRIC;
    }
    /**
     * @see {@link Meter.createHistogram}
     */
    createHistogram(_name, _options) {
        return NOOP_HISTOGRAM_METRIC;
    }
    /**
     * @see {@link Meter.createCounter}
     */
    createCounter(_name, _options) {
        return NOOP_COUNTER_METRIC;
    }
    /**
     * @see {@link Meter.createUpDownCounter}
     */
    createUpDownCounter(_name, _options) {
        return NOOP_UP_DOWN_COUNTER_METRIC;
    }
    /**
     * @see {@link Meter.createObservableGauge}
     */
    createObservableGauge(_name, _options) {
        return NOOP_OBSERVABLE_GAUGE_METRIC;
    }
    /**
     * @see {@link Meter.createObservableCounter}
     */
    createObservableCounter(_name, _options) {
        return NOOP_OBSERVABLE_COUNTER_METRIC;
    }
    /**
     * @see {@link Meter.createObservableUpDownCounter}
     */
    createObservableUpDownCounter(_name, _options) {
        return NOOP_OBSERVABLE_UP_DOWN_COUNTER_METRIC;
    }
    /**
     * @see {@link Meter.addBatchObservableCallback}
     */
    addBatchObservableCallback(_callback, _observables) { }
    /**
     * @see {@link Meter.removeBatchObservableCallback}
     */
    removeBatchObservableCallback(_callback) { }
}
class NoopMetric {
}
class NoopCounterMetric extends NoopMetric {
    add(_value, _attributes) { }
}
class NoopUpDownCounterMetric extends NoopMetric {
    add(_value, _attributes) { }
}
class NoopGaugeMetric extends NoopMetric {
    record(_value, _attributes) { }
}
class NoopHistogramMetric extends NoopMetric {
    record(_value, _attributes) { }
}
class NoopObservableMetric {
    addCallback(_callback) { }
    removeCallback(_callback) { }
}
class NoopObservableCounterMetric extends NoopObservableMetric {
}
class NoopObservableGaugeMetric extends NoopObservableMetric {
}
class NoopObservableUpDownCounterMetric extends NoopObservableMetric {
}
const NOOP_METER = new NoopMeter();
// Synchronous instruments
const NOOP_COUNTER_METRIC = new NoopCounterMetric();
const NOOP_GAUGE_METRIC = new NoopGaugeMetric();
const NOOP_HISTOGRAM_METRIC = new NoopHistogramMetric();
const NOOP_UP_DOWN_COUNTER_METRIC = new NoopUpDownCounterMetric();
// Asynchronous instruments
const NOOP_OBSERVABLE_COUNTER_METRIC = new NoopObservableCounterMetric();
const NOOP_OBSERVABLE_GAUGE_METRIC = new NoopObservableGaugeMetric();
const NOOP_OBSERVABLE_UP_DOWN_COUNTER_METRIC = new NoopObservableUpDownCounterMetric();
/**
 * Create a no-op Meter
 *
 * @since 1.3.0
 */
function createNoopMeter() {
    return NOOP_METER;
}
//# sourceMappingURL=NoopMeter.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api@1.9.1/node_modules/@opentelemetry/api/build/esm/metrics/NoopMeterProvider.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */

/**
 * An implementation of the {@link MeterProvider} which returns an impotent Meter
 * for all calls to `getMeter`
 */
class NoopMeterProvider {
    getMeter(_name, _version, _options) {
        return NOOP_METER;
    }
}
const NOOP_METER_PROVIDER = new NoopMeterProvider();
//# sourceMappingURL=NoopMeterProvider.js.map
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api@1.9.1/node_modules/@opentelemetry/api/build/esm/internal/global-utils.js + 2 modules
var global_utils = __webpack_require__(66475);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api@1.9.1/node_modules/@opentelemetry/api/build/esm/api/diag.js + 3 modules
var api_diag = __webpack_require__(53259);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api@1.9.1/node_modules/@opentelemetry/api/build/esm/api/metrics.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */



const API_NAME = 'metrics';
/**
 * Singleton object which represents the entry point to the OpenTelemetry Metrics API
 */
class MetricsAPI {
    /** Empty private constructor prevents end users from constructing a new instance of the API */
    constructor() { }
    /** Get the singleton instance of the Metrics API */
    static getInstance() {
        if (!this._instance) {
            this._instance = new MetricsAPI();
        }
        return this._instance;
    }
    /**
     * Set the current global meter provider.
     * Returns true if the meter provider was successfully registered, else false.
     */
    setGlobalMeterProvider(provider) {
        return (0,global_utils/* registerGlobal */.$G)(API_NAME, provider, api_diag/* DiagAPI */.K.instance());
    }
    /**
     * Returns the global meter provider.
     */
    getMeterProvider() {
        return (0,global_utils/* getGlobal */.mS)(API_NAME) || NOOP_METER_PROVIDER;
    }
    /**
     * Returns a meter from the global meter provider.
     */
    getMeter(name, version, options) {
        return this.getMeterProvider().getMeter(name, version, options);
    }
    /** Remove the global meter provider */
    disable() {
        (0,global_utils/* unregisterGlobal */.kv)(API_NAME, api_diag/* DiagAPI */.K.instance());
    }
}
//# sourceMappingURL=metrics.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api@1.9.1/node_modules/@opentelemetry/api/build/esm/metrics-api.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
// Split module-level variable definition into separate files to allow
// tree-shaking on each api instance.

/**
 * Entrypoint for metrics API
 *
 * @since 1.3.0
 */
const metrics = MetricsAPI.getInstance();
//# sourceMappingURL=metrics-api.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api-logs@0.222.0/node_modules/@opentelemetry/api-logs/build/esm/internal/global-utils.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
const GLOBAL_LOGS_API_KEY = Symbol.for('io.opentelemetry.js.api.logs');
const _global = globalThis;
/**
 * Make a function which accepts a version integer and returns the instance of an API if the version
 * is compatible, or a fallback version (usually NOOP) if it is not.
 *
 * @param requiredVersion Backwards compatibility version which is required to return the instance
 * @param instance Instance which should be returned if the required version is compatible
 * @param fallback Fallback instance, usually NOOP, which will be returned if the required version is not compatible
 */
function makeGetter(requiredVersion, instance, fallback) {
    return (version) => version === requiredVersion ? instance : fallback;
}
/**
 * A number which should be incremented each time a backwards incompatible
 * change is made to the API. This number is used when an API package
 * attempts to access the global API to ensure it is getting a compatible
 * version. If the global API is not compatible with the API package
 * attempting to get it, a NOOP API implementation will be returned.
 */
const API_BACKWARDS_COMPATIBILITY_VERSION = 1;
//# sourceMappingURL=global-utils.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api-logs@0.222.0/node_modules/@opentelemetry/api-logs/build/esm/NoopLogger.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
class NoopLogger {
    emit(_logRecord) { }
    enabled() {
        return false;
    }
}
const NOOP_LOGGER = new NoopLogger();
/**
 * Create a no-op Logger
 */
function createNoopLogger() {
    return NOOP_LOGGER;
}
//# sourceMappingURL=NoopLogger.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api-logs@0.222.0/node_modules/@opentelemetry/api-logs/build/esm/NoopLoggerProvider.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */

class NoopLoggerProvider {
    getLogger(_name, _version, _options) {
        return new NoopLogger();
    }
}
const NOOP_LOGGER_PROVIDER = new NoopLoggerProvider();
//# sourceMappingURL=NoopLoggerProvider.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api-logs@0.222.0/node_modules/@opentelemetry/api-logs/build/esm/ProxyLogger.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */

class ProxyLogger {
    constructor(provider, name, version, options) {
        this._provider = provider;
        this.name = name;
        this.version = version;
        this.options = options;
    }
    /**
     * Emit a log record. This method should only be used by log appenders.
     *
     * @param logRecord
     */
    emit(logRecord) {
        this._getLogger().emit(logRecord);
    }
    enabled(options) {
        return this._getLogger().enabled(options);
    }
    /**
     * Try to get a logger from the proxy logger provider.
     * If the proxy logger provider has no delegate, return a noop logger.
     */
    _getLogger() {
        if (this._delegate) {
            return this._delegate;
        }
        const logger = this._provider._getDelegateLogger(this.name, this.version, this.options);
        if (!logger) {
            return NOOP_LOGGER;
        }
        this._delegate = logger;
        return this._delegate;
    }
}
//# sourceMappingURL=ProxyLogger.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api-logs@0.222.0/node_modules/@opentelemetry/api-logs/build/esm/ProxyLoggerProvider.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */


class ProxyLoggerProvider {
    getLogger(name, version, options) {
        var _a;
        return ((_a = this._getDelegateLogger(name, version, options)) !== null && _a !== void 0 ? _a : new ProxyLogger(this, name, version, options));
    }
    /**
     * Get the delegate logger provider.
     * Used by tests only.
     * @internal
     */
    _getDelegate() {
        var _a;
        return (_a = this._delegate) !== null && _a !== void 0 ? _a : NOOP_LOGGER_PROVIDER;
    }
    /**
     * Set the delegate logger provider
     * @internal
     */
    _setDelegate(delegate) {
        this._delegate = delegate;
    }
    /**
     * @internal
     */
    _getDelegateLogger(name, version, options) {
        var _a;
        return (_a = this._delegate) === null || _a === void 0 ? void 0 : _a.getLogger(name, version, options);
    }
}
//# sourceMappingURL=ProxyLoggerProvider.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api-logs@0.222.0/node_modules/@opentelemetry/api-logs/build/esm/api/logs.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */



class LogsAPI {
    constructor() {
        this._proxyLoggerProvider = new ProxyLoggerProvider();
    }
    static getInstance() {
        if (!this._instance) {
            this._instance = new LogsAPI();
        }
        return this._instance;
    }
    setGlobalLoggerProvider(provider) {
        if (_global[GLOBAL_LOGS_API_KEY]) {
            return this.getLoggerProvider();
        }
        _global[GLOBAL_LOGS_API_KEY] = makeGetter(API_BACKWARDS_COMPATIBILITY_VERSION, provider, NOOP_LOGGER_PROVIDER);
        this._proxyLoggerProvider._setDelegate(provider);
        return provider;
    }
    /**
     * Returns the global logger provider.
     *
     * @returns LoggerProvider
     */
    getLoggerProvider() {
        var _a, _b;
        return ((_b = (_a = _global[GLOBAL_LOGS_API_KEY]) === null || _a === void 0 ? void 0 : _a.call(_global, API_BACKWARDS_COMPATIBILITY_VERSION)) !== null && _b !== void 0 ? _b : this._proxyLoggerProvider);
    }
    /**
     * Returns a Logger, creating one if one with the given name, version,
     * schemaUrl, and attributes is not already created.
     *
     * Getting a Logger may be expensive, especially when `attributes` are
     * provided. Reuse Logger instances where possible instead of calling
     * `getLogger()` on hot paths.
     *
     * @param name The name of the logger or instrumentation library.
     * @param version The version of the logger or instrumentation library.
     * @param options The options of the logger or instrumentation library.
     * @returns {@link Logger}
     */
    getLogger(name, version, options) {
        return this.getLoggerProvider().getLogger(name, version, options);
    }
    /** Remove the global logger provider */
    disable() {
        delete _global[GLOBAL_LOGS_API_KEY];
        this._proxyLoggerProvider = new ProxyLoggerProvider();
    }
}
//# sourceMappingURL=logs.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api-logs@0.222.0/node_modules/@opentelemetry/api-logs/build/esm/index.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */



const logs = LogsAPI.getInstance();
//# sourceMappingURL=index.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+instrumentation@0.222.0+4294d048f15f6cd7/node_modules/@opentelemetry/instrumentation/build/esm/autoLoaderUtils.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
/**
 * Enable instrumentations
 * @param instrumentations
 * @param tracerProvider
 * @param meterProvider
 */
function enableInstrumentations(instrumentations, tracerProvider, meterProvider, loggerProvider) {
    for (let i = 0, j = instrumentations.length; i < j; i++) {
        const instrumentation = instrumentations[i];
        if (tracerProvider) {
            instrumentation.setTracerProvider(tracerProvider);
        }
        if (meterProvider) {
            instrumentation.setMeterProvider(meterProvider);
        }
        if (loggerProvider && instrumentation.setLoggerProvider) {
            instrumentation.setLoggerProvider(loggerProvider);
        }
        // instrumentations have been already enabled during creation
        // so enable only if user prevented that by setting enabled to false
        // this is to prevent double enabling but when calling register all
        // instrumentations should be now enabled
        if (!instrumentation.getConfig().enabled) {
            instrumentation.enable();
        }
    }
}
/**
 * Disable instrumentations
 * @param instrumentations
 */
function disableInstrumentations(instrumentations) {
    instrumentations.forEach(instrumentation => instrumentation.disable());
}
//# sourceMappingURL=autoLoaderUtils.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+instrumentation@0.222.0+4294d048f15f6cd7/node_modules/@opentelemetry/instrumentation/build/esm/autoLoader.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */



/**
 * It will register instrumentations and plugins
 * @param options
 * @return returns function to unload instrumentation and plugins that were
 *   registered
 */
function registerInstrumentations(options) {
    const tracerProvider = options.tracerProvider || trace_api/* trace */.u.getTracerProvider();
    const meterProvider = options.meterProvider || metrics.getMeterProvider();
    const loggerProvider = options.loggerProvider || logs.getLoggerProvider();
    const instrumentations = options.instrumentations?.flat() ?? [];
    enableInstrumentations(instrumentations, tracerProvider, meterProvider, loggerProvider);
    return () => {
        disableInstrumentations(instrumentations);
    };
}
//# sourceMappingURL=autoLoader.js.map
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api@1.9.1/node_modules/@opentelemetry/api/build/esm/api/context.js + 1 modules
var context = __webpack_require__(67119);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api@1.9.1/node_modules/@opentelemetry/api/build/esm/context-api.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
// Split module-level variable definition into separate files to allow
// tree-shaking on each api instance.

/**
 * Entrypoint for context API
 * @since 1.0.0
 */
const context_api_context = context/* ContextAPI */._.getInstance();
//# sourceMappingURL=context-api.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api@1.9.1/node_modules/@opentelemetry/api/build/esm/propagation/NoopTextMapPropagator.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
/**
 * No-op implementations of {@link TextMapPropagator}.
 */
class NoopTextMapPropagator {
    /** Noop inject function does nothing */
    inject(_context, _carrier) { }
    /** Noop extract function does nothing and returns the input context */
    extract(context, _carrier) {
        return context;
    }
    fields() {
        return [];
    }
}
//# sourceMappingURL=NoopTextMapPropagator.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api@1.9.1/node_modules/@opentelemetry/api/build/esm/propagation/TextMapPropagator.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
/**
 * @since 1.0.0
 */
const defaultTextMapGetter = {
    get(carrier, key) {
        if (carrier == null) {
            return undefined;
        }
        return carrier[key];
    },
    keys(carrier) {
        if (carrier == null) {
            return [];
        }
        return Object.keys(carrier);
    },
};
/**
 * @since 1.0.0
 */
const defaultTextMapSetter = {
    set(carrier, key, value) {
        if (carrier == null) {
            return;
        }
        carrier[key] = value;
    },
};
//# sourceMappingURL=TextMapPropagator.js.map
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api@1.9.1/node_modules/@opentelemetry/api/build/esm/context/context.js
var context_context = __webpack_require__(32022);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api@1.9.1/node_modules/@opentelemetry/api/build/esm/baggage/context-helpers.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */


/**
 * Baggage key
 */
const BAGGAGE_KEY = (0,context_context/* createContextKey */.n)('OpenTelemetry Baggage Key');
/**
 * Retrieve the current baggage from the given context
 *
 * @param {Context} Context that manage all context values
 * @returns {Baggage} Extracted baggage from the context
 */
function getBaggage(context) {
    return context.getValue(BAGGAGE_KEY) || undefined;
}
/**
 * Retrieve the current baggage from the active/current context
 *
 * @returns {Baggage} Extracted baggage from the context
 */
function getActiveBaggage() {
    return getBaggage(context/* ContextAPI */._.getInstance().active());
}
/**
 * Store a baggage in the given context
 *
 * @param {Context} Context that manage all context values
 * @param {Baggage} baggage that will be set in the actual context
 */
function setBaggage(context, baggage) {
    return context.setValue(BAGGAGE_KEY, baggage);
}
/**
 * Delete the baggage stored in the given context
 *
 * @param {Context} Context that manage all context values
 */
function deleteBaggage(context) {
    return context.deleteValue(BAGGAGE_KEY);
}
//# sourceMappingURL=context-helpers.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api@1.9.1/node_modules/@opentelemetry/api/build/esm/baggage/internal/baggage-impl.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
class BaggageImpl {
    constructor(entries) {
        this._entries = entries ? new Map(entries) : new Map();
    }
    getEntry(key) {
        const entry = this._entries.get(key);
        if (!entry) {
            return undefined;
        }
        return Object.assign({}, entry);
    }
    getAllEntries() {
        return Array.from(this._entries.entries());
    }
    setEntry(key, entry) {
        const newBaggage = new BaggageImpl(this._entries);
        newBaggage._entries.set(key, entry);
        return newBaggage;
    }
    removeEntry(key) {
        const newBaggage = new BaggageImpl(this._entries);
        newBaggage._entries.delete(key);
        return newBaggage;
    }
    removeEntries(...keys) {
        const newBaggage = new BaggageImpl(this._entries);
        for (const key of keys) {
            newBaggage._entries.delete(key);
        }
        return newBaggage;
    }
    clear() {
        return new BaggageImpl();
    }
}
//# sourceMappingURL=baggage-impl.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api@1.9.1/node_modules/@opentelemetry/api/build/esm/baggage/internal/symbol.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
/**
 * Symbol used to make BaggageEntryMetadata an opaque type
 */
const baggageEntryMetadataSymbol = Symbol('BaggageEntryMetadata');
//# sourceMappingURL=symbol.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api@1.9.1/node_modules/@opentelemetry/api/build/esm/baggage/utils.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */



const utils_diag = api_diag/* DiagAPI */.K.instance();
/**
 * Create a new Baggage with optional entries
 *
 * @param entries An array of baggage entries the new baggage should contain
 */
function createBaggage(entries = {}) {
    return new BaggageImpl(new Map(Object.entries(entries)));
}
/**
 * Create a serializable BaggageEntryMetadata object from a string.
 *
 * @param str string metadata. Format is currently not defined by the spec and has no special meaning.
 *
 * @since 1.0.0
 */
function baggageEntryMetadataFromString(str) {
    if (typeof str !== 'string') {
        utils_diag.error(`Cannot create baggage metadata from unknown type: ${typeof str}`);
        str = '';
    }
    return {
        __TYPE__: baggageEntryMetadataSymbol,
        toString() {
            return str;
        },
    };
}
//# sourceMappingURL=utils.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api@1.9.1/node_modules/@opentelemetry/api/build/esm/api/propagation.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */






const propagation_API_NAME = 'propagation';
const NOOP_TEXT_MAP_PROPAGATOR = new NoopTextMapPropagator();
/**
 * Singleton object which represents the entry point to the OpenTelemetry Propagation API
 *
 * @since 1.0.0
 */
class PropagationAPI {
    /** Empty private constructor prevents end users from constructing a new instance of the API */
    constructor() {
        this.createBaggage = createBaggage;
        this.getBaggage = getBaggage;
        this.getActiveBaggage = getActiveBaggage;
        this.setBaggage = setBaggage;
        this.deleteBaggage = deleteBaggage;
    }
    /** Get the singleton instance of the Propagator API */
    static getInstance() {
        if (!this._instance) {
            this._instance = new PropagationAPI();
        }
        return this._instance;
    }
    /**
     * Set the current propagator.
     *
     * @returns true if the propagator was successfully registered, else false
     */
    setGlobalPropagator(propagator) {
        return (0,global_utils/* registerGlobal */.$G)(propagation_API_NAME, propagator, api_diag/* DiagAPI */.K.instance());
    }
    /**
     * Inject context into a carrier to be propagated inter-process
     *
     * @param context Context carrying tracing data to inject
     * @param carrier carrier to inject context into
     * @param setter Function used to set values on the carrier
     */
    inject(context, carrier, setter = defaultTextMapSetter) {
        return this._getGlobalPropagator().inject(context, carrier, setter);
    }
    /**
     * Extract context from a carrier
     *
     * @param context Context which the newly created context will inherit from
     * @param carrier Carrier to extract context from
     * @param getter Function used to extract keys from a carrier
     */
    extract(context, carrier, getter = defaultTextMapGetter) {
        return this._getGlobalPropagator().extract(context, carrier, getter);
    }
    /**
     * Return a list of all fields which may be used by the propagator.
     */
    fields() {
        return this._getGlobalPropagator().fields();
    }
    /** Remove the global propagator */
    disable() {
        (0,global_utils/* unregisterGlobal */.kv)(propagation_API_NAME, api_diag/* DiagAPI */.K.instance());
    }
    _getGlobalPropagator() {
        return (0,global_utils/* getGlobal */.mS)(propagation_API_NAME) || NOOP_TEXT_MAP_PROPAGATOR;
    }
}
//# sourceMappingURL=propagation.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api@1.9.1/node_modules/@opentelemetry/api/build/esm/propagation-api.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
// Split module-level variable definition into separate files to allow
// tree-shaking on each api instance.

/**
 * Entrypoint for propagation API
 *
 * @since 1.0.0
 */
const propagation = PropagationAPI.getInstance();
//# sourceMappingURL=propagation-api.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+core@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/core/build/esm/platform/node/index.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */



/**
 * @deprecated Use performance directly.
 */
const otperformance = performance;
//# sourceMappingURL=index.js.map
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api@1.9.1/node_modules/@opentelemetry/api/build/esm/trace/spancontext-utils.js
var spancontext_utils = __webpack_require__(24156);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api@1.9.1/node_modules/@opentelemetry/api/build/esm/trace/trace_flags.js
var trace_flags = __webpack_require__(51810);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+core@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/core/build/esm/trace/suppress-tracing.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */

const SUPPRESS_TRACING_KEY = (0,context_context/* createContextKey */.n)('OpenTelemetry SDK Context Key SUPPRESS_TRACING');
function suppressTracing(context) {
    return context.setValue(SUPPRESS_TRACING_KEY, true);
}
function unsuppressTracing(context) {
    return context.deleteValue(SUPPRESS_TRACING_KEY);
}
function isTracingSuppressed(context) {
    return context.getValue(SUPPRESS_TRACING_KEY) === true;
}
//# sourceMappingURL=suppress-tracing.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+core@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/core/build/esm/internal/validators.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
const VALID_KEY_CHAR_RANGE = '[_0-9a-z-*/]';
const VALID_KEY = `[a-z]${VALID_KEY_CHAR_RANGE}{0,255}`;
const VALID_VENDOR_KEY = `[a-z0-9]${VALID_KEY_CHAR_RANGE}{0,240}@[a-z]${VALID_KEY_CHAR_RANGE}{0,13}`;
const VALID_KEY_REGEX = new RegExp(`^(?:${VALID_KEY}|${VALID_VENDOR_KEY})$`);
const VALID_VALUE_BASE_REGEX = /^[ -~]{0,255}[!-~]$/;
const INVALID_VALUE_COMMA_EQUAL_REGEX = /,|=/;
/**
 * Key is opaque string up to 256 characters printable. It MUST begin with a
 * lowercase letter, and can only contain lowercase letters a-z, digits 0-9,
 * underscores _, dashes -, asterisks *, and forward slashes /.
 * For multi-tenant vendor scenarios, an at sign (@) can be used to prefix the
 * vendor name. Vendors SHOULD set the tenant ID at the beginning of the key.
 * see https://www.w3.org/TR/trace-context/#key
 */
function validateKey(key) {
    return VALID_KEY_REGEX.test(key);
}
/**
 * Value is opaque string up to 256 characters printable ASCII RFC0020
 * characters (i.e., the range 0x20 to 0x7E) except comma , and =.
 */
function validateValue(value) {
    return (VALID_VALUE_BASE_REGEX.test(value) &&
        !INVALID_VALUE_COMMA_EQUAL_REGEX.test(value));
}
//# sourceMappingURL=validators.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+core@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/core/build/esm/trace/TraceState.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */

const MAX_TRACE_STATE_ITEMS = 32;
const MAX_TRACE_STATE_LEN = 512;
const LIST_MEMBERS_SEPARATOR = ',';
const LIST_MEMBER_KEY_VALUE_SPLITTER = '=';
/**
 * TraceState must be a class and not a simple object type because of the spec
 * requirement (https://www.w3.org/TR/trace-context/#tracestate-field).
 *
 * Here is the list of allowed mutations:
 * - New key-value pair should be added into the beginning of the list
 * - The value of any key can be updated. Modified keys MUST be moved to the
 * beginning of the list.
 */
class TraceState {
    _length;
    _rawTraceState;
    _internalState;
    constructor(rawTraceState) {
        this._rawTraceState =
            typeof rawTraceState === 'string' ? rawTraceState : '';
        this._length = this._rawTraceState.length;
    }
    set(key, value) {
        if (!validateKey(key) || !validateValue(value)) {
            return this;
        }
        const currState = this._getState();
        const currValue = currState.get(key);
        // Get the new length depending if we already have a value or not
        // - for existing keys we add the difference between the length of the values
        // - for new keys is the key & value lenght plus
        //   - +1 for the key/value splitter
        //   - +1 for the separator if there are other keys
        let newLength = this._length;
        if (typeof currValue === 'string') {
            newLength += value.length - currValue.length;
        }
        else {
            newLength += key.length + value.length + (currState.size > 0 ? 2 : 1);
        }
        if (newLength > MAX_TRACE_STATE_LEN) {
            return this;
        }
        const newState = new Map(currState);
        newState.delete(key);
        newState.set(key, value);
        return this._fromState(newState, newLength);
    }
    unset(key) {
        const currState = this._getState();
        const currValue = currState.get(key);
        // No need to create a new instance if the key does not exist
        if (typeof currValue !== 'string') {
            return this;
        }
        // Get the new length depending if we already have a value or not
        // - for existing keys we substract key and value length plus
        //   - +1 for the key/value splitter
        //   - +1 for the separator if there are other keys
        let newLength = this._length - (key.length + currValue.length + 1);
        if (currState.size > 1) {
            // remove separator from length if there's no key or only one.
            newLength = newLength - 1;
        }
        const newState = new Map(currState);
        newState.delete(key);
        return this._fromState(newState, newLength);
    }
    get(key) {
        const currState = this._getState();
        return currState.get(key);
    }
    serialize() {
        // Maps put new entries at the end. We prepend the seralized entry
        // to get the right order according to the spec (updated members go 1st)
        let serialized = '';
        let index = 0;
        for (const entry of this._getState()) {
            if (index > 0) {
                serialized = LIST_MEMBERS_SEPARATOR + serialized;
            }
            serialized =
                `${entry[0]}${LIST_MEMBER_KEY_VALUE_SPLITTER}${entry[1]}` + serialized;
            index++;
        }
        return serialized;
    }
    _getState() {
        if (this._internalState) {
            return this._internalState;
        }
        // Not parsed yet, lets do it
        const vendorMembers = this._rawTraceState.split(LIST_MEMBERS_SEPARATOR);
        // This Map will have the order reversed
        const vendorEntries = new Map();
        let currentLength = 0;
        for (const member of vendorMembers) {
            const m = member.trim();
            const idx = m.indexOf(LIST_MEMBER_KEY_VALUE_SPLITTER);
            if (idx === -1) {
                continue;
            }
            const key = m.slice(0, idx);
            const value = m.slice(idx + 1);
            if (!validateKey(key) || !validateValue(value)) {
                continue;
            }
            // Skip if adding the new member exceeds the length
            const futureLength = currentLength + m.length + (vendorEntries.size > 0 ? 1 : 0);
            if (futureLength > MAX_TRACE_STATE_LEN) {
                continue;
            }
            // All good, add it
            vendorEntries.set(key, value);
            currentLength = futureLength;
            // Check if we reached the max items
            if (vendorEntries.size >= MAX_TRACE_STATE_ITEMS) {
                break;
            }
        }
        // Now we set the length & the Map in the right order
        this._length = currentLength;
        this._internalState = new Map(Array.from(vendorEntries.entries()).reverse());
        return this._internalState;
    }
    _fromState(state, length) {
        const traceState = Object.create(TraceState.prototype);
        traceState._internalState = state;
        traceState._length = length;
        return traceState;
    }
}
//# sourceMappingURL=TraceState.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+core@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/core/build/esm/trace/W3CTraceContextPropagator.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */



const TRACE_PARENT_HEADER = 'traceparent';
const TRACE_STATE_HEADER = 'tracestate';
const VERSION = '00';
const VERSION_PART = '(?!ff)[\\da-f]{2}';
const TRACE_ID_PART = '(?![0]{32})[\\da-f]{32}';
const PARENT_ID_PART = '(?![0]{16})[\\da-f]{16}';
const FLAGS_PART = '[\\da-f]{2}';
const TRACE_PARENT_REGEX = new RegExp(`^\\s?(${VERSION_PART})-(${TRACE_ID_PART})-(${PARENT_ID_PART})-(${FLAGS_PART})(-.*)?\\s?$`);
/**
 * Parses information from the [traceparent] span tag and converts it into {@link SpanContext}
 * @param traceParent - A meta property that comes from server.
 *     It should be dynamically generated server side to have the server's request trace Id,
 *     a parent span Id that was set on the server's request span,
 *     and the trace flags to indicate the server's sampling decision
 *     (01 = sampled, 00 = not sampled).
 *     for example: '{version}-{traceId}-{spanId}-{sampleDecision}'
 *     For more information see {@link https://www.w3.org/TR/trace-context/}
 */
function parseTraceParent(traceParent) {
    const match = TRACE_PARENT_REGEX.exec(traceParent);
    if (!match)
        return null;
    // According to the specification the implementation should be compatible
    // with future versions. If there are more parts, we only reject it if it's using version 00
    // See https://www.w3.org/TR/trace-context/#versioning-of-traceparent
    if (match[1] === '00' && match[5])
        return null;
    return {
        traceId: match[2],
        spanId: match[3],
        traceFlags: parseInt(match[4], 16),
    };
}
/**
 * Propagates {@link SpanContext} through Trace Context format propagation.
 *
 * Based on the Trace Context specification:
 * https://www.w3.org/TR/trace-context/
 */
class W3CTraceContextPropagator {
    inject(context, carrier, setter) {
        const spanContext = trace_api/* trace */.u.getSpanContext(context);
        if (!spanContext ||
            isTracingSuppressed(context) ||
            !(0,spancontext_utils/* isSpanContextValid */.YA)(spanContext))
            return;
        const traceParent = `${VERSION}-${spanContext.traceId}-${spanContext.spanId}-0${Number(spanContext.traceFlags || trace_flags/* TraceFlags */.X.NONE).toString(16)}`;
        setter.set(carrier, TRACE_PARENT_HEADER, traceParent);
        if (spanContext.traceState) {
            setter.set(carrier, TRACE_STATE_HEADER, spanContext.traceState.serialize());
        }
    }
    extract(context, carrier, getter) {
        const traceParentHeader = getter.get(carrier, TRACE_PARENT_HEADER);
        if (!traceParentHeader)
            return context;
        const traceParent = Array.isArray(traceParentHeader)
            ? traceParentHeader[0]
            : traceParentHeader;
        if (typeof traceParent !== 'string')
            return context;
        const spanContext = parseTraceParent(traceParent);
        if (!spanContext)
            return context;
        spanContext.isRemote = true;
        const traceStateHeader = getter.get(carrier, TRACE_STATE_HEADER);
        if (traceStateHeader) {
            // If more than one `tracestate` header is found, we merge them into a
            // single header.
            const state = Array.isArray(traceStateHeader)
                ? traceStateHeader.join(',')
                : traceStateHeader;
            spanContext.traceState = new TraceState(typeof state === 'string' ? state : undefined);
        }
        return trace_api/* trace */.u.setSpanContext(context, spanContext);
    }
    fields() {
        return [TRACE_PARENT_HEADER, TRACE_STATE_HEADER];
    }
}
//# sourceMappingURL=W3CTraceContextPropagator.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+sdk-trace-web@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/sdk-trace-web/build/esm/enums/PerformanceTimingNames.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
var PerformanceTimingNames;
(function (PerformanceTimingNames) {
    PerformanceTimingNames["CONNECT_END"] = "connectEnd";
    PerformanceTimingNames["CONNECT_START"] = "connectStart";
    PerformanceTimingNames["DECODED_BODY_SIZE"] = "decodedBodySize";
    PerformanceTimingNames["DOM_COMPLETE"] = "domComplete";
    PerformanceTimingNames["DOM_CONTENT_LOADED_EVENT_END"] = "domContentLoadedEventEnd";
    PerformanceTimingNames["DOM_CONTENT_LOADED_EVENT_START"] = "domContentLoadedEventStart";
    PerformanceTimingNames["DOM_INTERACTIVE"] = "domInteractive";
    PerformanceTimingNames["DOMAIN_LOOKUP_END"] = "domainLookupEnd";
    PerformanceTimingNames["DOMAIN_LOOKUP_START"] = "domainLookupStart";
    PerformanceTimingNames["ENCODED_BODY_SIZE"] = "encodedBodySize";
    PerformanceTimingNames["FETCH_START"] = "fetchStart";
    PerformanceTimingNames["LOAD_EVENT_END"] = "loadEventEnd";
    PerformanceTimingNames["LOAD_EVENT_START"] = "loadEventStart";
    PerformanceTimingNames["NAVIGATION_START"] = "navigationStart";
    PerformanceTimingNames["REDIRECT_END"] = "redirectEnd";
    PerformanceTimingNames["REDIRECT_START"] = "redirectStart";
    PerformanceTimingNames["REQUEST_START"] = "requestStart";
    PerformanceTimingNames["RESPONSE_END"] = "responseEnd";
    PerformanceTimingNames["RESPONSE_START"] = "responseStart";
    PerformanceTimingNames["SECURE_CONNECTION_START"] = "secureConnectionStart";
    PerformanceTimingNames["START_TIME"] = "startTime";
    PerformanceTimingNames["UNLOAD_EVENT_END"] = "unloadEventEnd";
    PerformanceTimingNames["UNLOAD_EVENT_START"] = "unloadEventStart";
})(PerformanceTimingNames || (PerformanceTimingNames = {}));
//# sourceMappingURL=PerformanceTimingNames.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+core@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/core/build/esm/common/time.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */

const NANOSECOND_DIGITS = 9;
const NANOSECOND_DIGITS_IN_MILLIS = 6;
const MILLISECONDS_TO_NANOSECONDS = Math.pow(10, NANOSECOND_DIGITS_IN_MILLIS);
const SECOND_TO_NANOSECONDS = Math.pow(10, NANOSECOND_DIGITS);
/**
 * Converts a number of milliseconds from epoch to HrTime([seconds, remainder in nanoseconds]).
 * @param epochMillis
 */
function millisToHrTime(epochMillis) {
    const epochSeconds = epochMillis / 1000;
    // Decimals only.
    const seconds = Math.trunc(epochSeconds);
    // Round sub-nanosecond accuracy to nanosecond.
    const nanos = Math.round((epochMillis % 1000) * MILLISECONDS_TO_NANOSECONDS);
    return [seconds, nanos];
}
/**
 * @deprecated Use `performance.timeOrigin` directly.
 */
function getTimeOrigin() {
    return performance.timeOrigin;
}
/**
 * Returns an hrtime calculated via performance component.
 * @param performanceNow
 */
function hrTime(performanceNow) {
    const timeOrigin = millisToHrTime(otperformance.timeOrigin);
    const now = millisToHrTime(typeof performanceNow === 'number' ? performanceNow : otperformance.now());
    return addHrTimes(timeOrigin, now);
}
/**
 *
 * Converts a TimeInput to an HrTime, defaults to _hrtime().
 * @param time
 */
function timeInputToHrTime(time) {
    // process.hrtime
    if (isTimeInputHrTime(time)) {
        return time;
    }
    else if (typeof time === 'number') {
        // Distinguish between a relative performance.now() value and an absolute
        // epoch-millisecond timestamp.
        //
        // performance.timeOrigin uses a monotonic clock that may be slightly ahead
        // of Date.now() due to clock adjustments (see MDN docs). This means a
        // real epoch-ms value (e.g. Date.now()) can land just below
        // performance.timeOrigin, causing it to be misclassified as a relative
        // performance.now() reading and doubled when timeOrigin is added.
        //
        // A genuine performance.now() value represents elapsed time since the
        // document/process started, so it is bounded by system uptime. No real
        // system has been running long enough for performance.now() to reach half
        // the current epoch time (~1994 in ms terms). We therefore treat any value
        // below half of performance.timeOrigin as a relative performance.now()
        // reading, and everything else as an absolute epoch-millisecond timestamp.
        if (time < otperformance.timeOrigin / 2) {
            return hrTime(time);
        }
        else {
            // epoch milliseconds or performance.timeOrigin
            return millisToHrTime(time);
        }
    }
    else if (time instanceof Date) {
        return millisToHrTime(time.getTime());
    }
    else {
        throw TypeError('Invalid input type');
    }
}
/**
 * Returns a duration of two hrTime.
 * @param startTime
 * @param endTime
 */
function hrTimeDuration(startTime, endTime) {
    let seconds = endTime[0] - startTime[0];
    let nanos = endTime[1] - startTime[1];
    // overflow
    if (nanos < 0) {
        seconds -= 1;
        // negate
        nanos += SECOND_TO_NANOSECONDS;
    }
    return [seconds, nanos];
}
/**
 * Convert hrTime to timestamp, for example "2019-05-14T17:00:00.000123456Z"
 * @param time
 */
function hrTimeToTimeStamp(time) {
    const precision = NANOSECOND_DIGITS;
    const tmp = `${'0'.repeat(precision)}${time[1]}Z`;
    const nanoString = tmp.substring(tmp.length - precision - 1);
    const date = new Date(time[0] * 1000).toISOString();
    return date.replace('000Z', nanoString);
}
/**
 * Convert hrTime to nanoseconds.
 * @param time
 */
function hrTimeToNanoseconds(time) {
    return time[0] * SECOND_TO_NANOSECONDS + time[1];
}
/**
 * Convert hrTime to microseconds.
 * @param time
 */
function hrTimeToMicroseconds(time) {
    return time[0] * 1e6 + time[1] / 1e3;
}
/**
 * Convert hrTime to milliseconds.
 * @param time
 */
function hrTimeToMilliseconds(time) {
    return time[0] * 1e3 + time[1] / 1e6;
}
/**
 * Convert hrTime to seconds.
 * @param time
 */
function hrTimeToSeconds(time) {
    return time[0] + time[1] / SECOND_TO_NANOSECONDS;
}
/**
 * check if time is HrTime
 * @param value
 */
function isTimeInputHrTime(value) {
    return (Array.isArray(value) &&
        value.length === 2 &&
        typeof value[0] === 'number' &&
        typeof value[1] === 'number');
}
/**
 * check if input value is a correct types.TimeInput
 * @param value
 */
function isTimeInput(value) {
    return (isTimeInputHrTime(value) ||
        typeof value === 'number' ||
        value instanceof Date);
}
/**
 * Given 2 HrTime formatted times, return their sum as an HrTime.
 */
function addHrTimes(time1, time2) {
    const out = [time1[0] + time2[0], time1[1] + time2[1]];
    // Nanoseconds
    if (out[1] >= SECOND_TO_NANOSECONDS) {
        out[1] -= SECOND_TO_NANOSECONDS;
        out[0] += 1;
    }
    return out;
}
//# sourceMappingURL=time.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+core@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/core/build/esm/utils/url.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
function urlMatches(url, urlToMatch) {
    if (typeof urlToMatch === 'string') {
        return url === urlToMatch;
    }
    else {
        return !!url.match(urlToMatch);
    }
}
/**
 * Check if {@param url} should be ignored when comparing against {@param ignoredUrls}
 * @param url
 * @param ignoredUrls
 */
function isUrlIgnored(url, ignoredUrls) {
    if (!ignoredUrls) {
        return false;
    }
    for (const ignoreUrl of ignoredUrls) {
        if (urlMatches(url, ignoreUrl)) {
            return true;
        }
    }
    return false;
}
//# sourceMappingURL=url.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+sdk-trace-web@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/sdk-trace-web/build/esm/semconv.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
/*
 * This file contains a copy of unstable semantic convention definitions
 * used by this package.
 * @see https://github.com/open-telemetry/opentelemetry-js/tree/main/semantic-conventions#unstable-semconv
 */
/**
 * Deprecated, use `http.response.header.<key>` instead.
 *
 * @example 3495
 *
 * @experimental This attribute is experimental and is subject to breaking changes in minor releases of `@opentelemetry/semantic-conventions`.
 *
 * @deprecated Replaced by `http.response.header.<key>`.
 */
const ATTR_HTTP_RESPONSE_CONTENT_LENGTH = 'http.response_content_length';
/**
 * Deprecated, use `http.response.body.size` instead.
 *
 * @example 5493
 *
 * @experimental This attribute is experimental and is subject to breaking changes in minor releases of `@opentelemetry/semantic-conventions`.
 *
 * @deprecated Replace by `http.response.body.size`.
 */
const ATTR_HTTP_RESPONSE_CONTENT_LENGTH_UNCOMPRESSED = 'http.response_content_length_uncompressed';
//# sourceMappingURL=semconv.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+sdk-trace-web@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/sdk-trace-web/build/esm/utils.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */



// Used to normalize relative URLs
let urlNormalizingAnchor;
function getUrlNormalizingAnchor() {
    if (!urlNormalizingAnchor) {
        urlNormalizingAnchor = document.createElement('a');
    }
    return urlNormalizingAnchor;
}
/**
 * Helper function to be able to use enum as typed key in type and in interface when using forEach
 * @param obj
 * @param key
 */
function hasKey(obj, key) {
    return key in obj;
}
/**
 * Helper function for starting an event on span based on {@link PerformanceEntries}
 * @param span
 * @param performanceName name of performance entry for time start
 * @param entries
 * @param ignoreZeros
 */
function addSpanNetworkEvent(span, performanceName, entries, ignoreZeros = true) {
    if (hasKey(entries, performanceName) &&
        typeof entries[performanceName] === 'number' &&
        !(ignoreZeros && entries[performanceName] === 0)) {
        return span.addEvent(performanceName, entries[performanceName]);
    }
    return undefined;
}
/**
 * Helper function for adding network events and content length attributes.
 */
function addSpanNetworkEvents(span, resource, ignoreNetworkEvents = false, ignoreZeros, skipOldSemconvContentLengthAttrs) {
    if (ignoreZeros === undefined) {
        ignoreZeros = resource[PerformanceTimingNames.START_TIME] !== 0;
    }
    if (!ignoreNetworkEvents) {
        addSpanNetworkEvent(span, PerformanceTimingNames.FETCH_START, resource, ignoreZeros);
        addSpanNetworkEvent(span, PerformanceTimingNames.DOMAIN_LOOKUP_START, resource, ignoreZeros);
        addSpanNetworkEvent(span, PerformanceTimingNames.DOMAIN_LOOKUP_END, resource, ignoreZeros);
        addSpanNetworkEvent(span, PerformanceTimingNames.CONNECT_START, resource, ignoreZeros);
        addSpanNetworkEvent(span, PerformanceTimingNames.SECURE_CONNECTION_START, resource, ignoreZeros);
        addSpanNetworkEvent(span, PerformanceTimingNames.CONNECT_END, resource, ignoreZeros);
        addSpanNetworkEvent(span, PerformanceTimingNames.REQUEST_START, resource, ignoreZeros);
        addSpanNetworkEvent(span, PerformanceTimingNames.RESPONSE_START, resource, ignoreZeros);
        addSpanNetworkEvent(span, PerformanceTimingNames.RESPONSE_END, resource, ignoreZeros);
    }
    if (!skipOldSemconvContentLengthAttrs) {
        // This block adds content-length-related span attributes using the
        // *old* HTTP semconv (v1.7.0).
        const encodedLength = resource[PerformanceTimingNames.ENCODED_BODY_SIZE];
        if (encodedLength !== undefined) {
            span.setAttribute(ATTR_HTTP_RESPONSE_CONTENT_LENGTH, encodedLength);
        }
        const decodedLength = resource[PerformanceTimingNames.DECODED_BODY_SIZE];
        // Spec: Not set if transport encoding not used (in which case encoded and decoded sizes match)
        if (decodedLength !== undefined && encodedLength !== decodedLength) {
            span.setAttribute(ATTR_HTTP_RESPONSE_CONTENT_LENGTH_UNCOMPRESSED, decodedLength);
        }
    }
}
/**
 * sort resources by startTime
 * @param filteredResources
 */
function sortResources(filteredResources) {
    return filteredResources.slice().sort((a, b) => {
        const valueA = a[PerformanceTimingNames.FETCH_START];
        const valueB = b[PerformanceTimingNames.FETCH_START];
        if (valueA > valueB) {
            return 1;
        }
        else if (valueA < valueB) {
            return -1;
        }
        return 0;
    });
}
/** Returns the origin if present (if in browser context). */
function getOrigin() {
    return typeof location !== 'undefined' ? location.origin : undefined;
}
/**
 * Get closest performance resource ignoring the resources that have been
 * already used.
 * @param spanUrl
 * @param startTimeHR
 * @param endTimeHR
 * @param resources
 * @param ignoredResources
 * @param initiatorType
 */
function getResource(spanUrl, startTimeHR, endTimeHR, resources, ignoredResources = new WeakSet(), initiatorType) {
    // de-relativize the URL before usage (does no harm to absolute URLs)
    const parsedSpanUrl = parseUrl(spanUrl);
    spanUrl = parsedSpanUrl.toString();
    const filteredResources = filterResourcesForSpan(spanUrl, startTimeHR, endTimeHR, resources, ignoredResources, initiatorType);
    if (filteredResources.length === 0) {
        return {
            mainRequest: undefined,
        };
    }
    if (filteredResources.length === 1) {
        return {
            mainRequest: filteredResources[0],
        };
    }
    const sorted = sortResources(filteredResources);
    if (parsedSpanUrl.origin !== getOrigin() && sorted.length > 1) {
        let corsPreFlightRequest = sorted[0];
        let mainRequest = findMainRequest(sorted, corsPreFlightRequest[PerformanceTimingNames.RESPONSE_END], endTimeHR);
        const responseEnd = corsPreFlightRequest[PerformanceTimingNames.RESPONSE_END];
        const fetchStart = mainRequest[PerformanceTimingNames.FETCH_START];
        // no corsPreFlightRequest
        if (fetchStart < responseEnd) {
            mainRequest = corsPreFlightRequest;
            corsPreFlightRequest = undefined;
        }
        return {
            corsPreFlightRequest,
            mainRequest,
        };
    }
    else {
        return {
            mainRequest: filteredResources[0],
        };
    }
}
/**
 * Will find the main request skipping the cors pre flight requests
 * @param resources
 * @param corsPreFlightRequestEndTime
 * @param spanEndTimeHR
 */
function findMainRequest(resources, corsPreFlightRequestEndTime, spanEndTimeHR) {
    const spanEndTime = hrTimeToNanoseconds(spanEndTimeHR);
    const minTime = hrTimeToNanoseconds(timeInputToHrTime(corsPreFlightRequestEndTime));
    let mainRequest = resources[1];
    let bestGap;
    const length = resources.length;
    for (let i = 1; i < length; i++) {
        const resource = resources[i];
        const resourceStartTime = hrTimeToNanoseconds(timeInputToHrTime(resource[PerformanceTimingNames.FETCH_START]));
        const resourceEndTime = hrTimeToNanoseconds(timeInputToHrTime(resource[PerformanceTimingNames.RESPONSE_END]));
        const currentGap = spanEndTime - resourceEndTime;
        if (resourceStartTime >= minTime && (!bestGap || currentGap < bestGap)) {
            bestGap = currentGap;
            mainRequest = resource;
        }
    }
    return mainRequest;
}
/**
 * Filter all resources that has started and finished according to span start time and end time.
 *     It will return the closest resource to a start time
 * @param spanUrl
 * @param startTimeHR
 * @param endTimeHR
 * @param resources
 * @param ignoredResources
 */
function filterResourcesForSpan(spanUrl, startTimeHR, endTimeHR, resources, ignoredResources, initiatorType) {
    const startTime = hrTimeToNanoseconds(startTimeHR);
    const endTime = hrTimeToNanoseconds(endTimeHR);
    let filteredResources = resources.filter(resource => {
        const resourceStartTime = hrTimeToNanoseconds(timeInputToHrTime(resource[PerformanceTimingNames.FETCH_START]));
        const resourceEndTime = hrTimeToNanoseconds(timeInputToHrTime(resource[PerformanceTimingNames.RESPONSE_END]));
        return (resource.initiatorType.toLowerCase() ===
            (initiatorType || 'xmlhttprequest') &&
            resource.name === spanUrl &&
            resourceStartTime >= startTime &&
            resourceEndTime <= endTime);
    });
    if (filteredResources.length > 0) {
        filteredResources = filteredResources.filter(resource => {
            return !ignoredResources.has(resource);
        });
    }
    return filteredResources;
}
/**
 * Parses url using URL constructor or fallback to anchor element.
 * @param url
 */
function parseUrl(url) {
    if (typeof URL === 'function') {
        return new URL(url, typeof document !== 'undefined'
            ? document.baseURI
            : typeof location !== 'undefined' // Some JS runtimes (e.g. Deno) don't define this
                ? location.href
                : undefined);
    }
    const element = getUrlNormalizingAnchor();
    element.href = url;
    return element;
}
/**
 * Parses url using URL constructor or fallback to anchor element and serialize
 * it to a string.
 *
 * Performs the steps described in https://html.spec.whatwg.org/multipage/urls-and-fetching.html#parse-a-url
 *
 * @param url
 */
function normalizeUrl(url) {
    const urlLike = parseUrl(url);
    return urlLike.href;
}
/**
 * Get element XPath
 * @param target - target element
 * @param optimised - when id attribute of element is present the xpath can be
 * simplified to contain id
 */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
function getElementXPath(target, optimised) {
    if (target.nodeType === Node.DOCUMENT_NODE) {
        return '/';
    }
    const targetValue = getNodeValue(target, optimised);
    if (optimised && targetValue.indexOf('@id') > 0) {
        return targetValue;
    }
    let xpath = '';
    if (target.parentNode) {
        xpath += getElementXPath(target.parentNode, optimised);
    }
    xpath += targetValue;
    return xpath;
}
/**
 * get node index within the siblings
 * @param target
 */
function getNodeIndex(target) {
    if (!target.parentNode) {
        return 0;
    }
    const allowedTypes = [target.nodeType];
    if (target.nodeType === Node.CDATA_SECTION_NODE) {
        allowedTypes.push(Node.TEXT_NODE);
    }
    let elements = Array.from(target.parentNode.childNodes);
    elements = elements.filter((element) => {
        const localName = element.localName;
        return (allowedTypes.indexOf(element.nodeType) >= 0 &&
            localName === target.localName);
    });
    if (elements.length >= 1) {
        return elements.indexOf(target) + 1; // xpath starts from 1
    }
    // if there are no other similar child xpath doesn't need index
    return 0;
}
/**
 * get node value for xpath
 * @param target
 * @param optimised
 */
function getNodeValue(target, optimised) {
    const nodeType = target.nodeType;
    const index = getNodeIndex(target);
    let nodeValue;
    if (nodeType === Node.ELEMENT_NODE) {
        const id = target.getAttribute('id');
        if (optimised && id) {
            return `//*[@id="${id}"]`;
        }
        nodeValue = target.localName;
    }
    else if (nodeType === Node.TEXT_NODE ||
        nodeType === Node.CDATA_SECTION_NODE) {
        nodeValue = 'text()';
    }
    else if (nodeType === Node.COMMENT_NODE) {
        nodeValue = 'comment()';
    }
    else {
        return '';
    }
    // if index is 1 it can be omitted in xpath
    if (nodeValue && index > 1) {
        return `/${nodeValue}[${index}]`;
    }
    return `/${nodeValue}`;
}
/**
 * Checks if trace headers should be propagated
 * @param spanUrl
 * @private
 */
function shouldPropagateTraceHeaders(spanUrl, propagateTraceHeaderCorsUrls) {
    let propagateTraceHeaderUrls = propagateTraceHeaderCorsUrls || [];
    if (typeof propagateTraceHeaderUrls === 'string' ||
        propagateTraceHeaderUrls instanceof RegExp) {
        propagateTraceHeaderUrls = [propagateTraceHeaderUrls];
    }
    const parsedSpanUrl = parseUrl(spanUrl);
    if (parsedSpanUrl.origin === getOrigin()) {
        return true;
    }
    else {
        return propagateTraceHeaderUrls.some(propagateTraceHeaderUrl => urlMatches(spanUrl, propagateTraceHeaderUrl));
    }
}
//# sourceMappingURL=utils.js.map
// EXTERNAL MODULE: external "path"
var external_path_ = __webpack_require__(33873);
// EXTERNAL MODULE: external "util"
var external_util_ = __webpack_require__(28354);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api@1.9.1/node_modules/@opentelemetry/api/build/esm/diag-api.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
// Split module-level variable definition into separate files to allow
// tree-shaking on each api instance.

/**
 * Entrypoint for Diag API.
 * Defines Diagnostic handler used for internal diagnostic logging operations.
 * The default provides a Noop DiagLogger implementation which may be changed via the
 * diag.setLogger(logger: DiagLogger) function.
 *
 * @since 1.0.0
 */
const diag_api_diag = api_diag/* DiagAPI */.K.instance();
//# sourceMappingURL=diag-api.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+instrumentation@0.222.0+4294d048f15f6cd7/node_modules/@opentelemetry/instrumentation/build/esm/semver.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
// This is a custom semantic versioning implementation compatible with the
// `satisfies(version, range, options?)` function from the `semver` npm package;
// with the exception that the `loose` option is not supported.
//
// The motivation for the custom semver implementation is that
// `semver` package has some initialization delay (lots of RegExp init and compile)
// and this leads to coldstart overhead for the OTEL Lambda Node.js layer.
// Hence, we have implemented lightweight version of it internally with required functionalities.

const VERSION_REGEXP = /^(?:v)?(?<version>(?<major>0|[1-9]\d*)\.(?<minor>0|[1-9]\d*)\.(?<patch>0|[1-9]\d*))(?:-(?<prerelease>(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\+(?<build>[0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$/;
const RANGE_REGEXP = /^(?<op><|>|=|==|<=|>=|~|\^|~>)?\s*(?:v)?(?<version>(?<major>x|X|\*|0|[1-9]\d*)(?:\.(?<minor>x|X|\*|0|[1-9]\d*))?(?:\.(?<patch>x|X|\*|0|[1-9]\d*))?)(?:-(?<prerelease>(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\+(?<build>[0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$/;
const operatorResMap = {
    '>': [1],
    '>=': [0, 1],
    '=': [0],
    '<=': [-1, 0],
    '<': [-1],
    '!=': [-1, 1],
};
/**
 * Checks given version whether it satisfies given range expression.
 * @param version the [version](https://github.com/npm/node-semver#versions) to be checked
 * @param range   the [range](https://github.com/npm/node-semver#ranges) expression for version check
 * @param options options to configure semver satisfy check
 */
function satisfies(version, range, options) {
    // Strict semver format check
    if (!_validateVersion(version)) {
        diag_api_diag.error(`Invalid version: ${version}`);
        return false;
    }
    // If range is empty, satisfy check succeeds regardless what version is
    if (!range) {
        return true;
    }
    // Cleanup range
    range = range.replace(/([<>=~^]+)\s+/g, '$1');
    // Parse version
    const parsedVersion = _parseVersion(version);
    if (!parsedVersion) {
        return false;
    }
    const allParsedRanges = [];
    // Check given version whether it satisfies given range expression
    const checkResult = _doSatisfies(parsedVersion, range, allParsedRanges, options);
    // If check result is OK,
    // do another final check for pre-release, if pre-release check is included by option
    if (checkResult && !options?.includePrerelease) {
        return _doPreleaseCheck(parsedVersion, allParsedRanges);
    }
    return checkResult;
}
function _validateVersion(version) {
    return typeof version === 'string' && VERSION_REGEXP.test(version);
}
function _doSatisfies(parsedVersion, range, allParsedRanges, options) {
    if (range.includes('||')) {
        // A version matches a range if and only if
        // every comparator in at least one of the ||-separated comparator sets is satisfied by the version
        const ranges = range.trim().split('||');
        for (const r of ranges) {
            if (_checkRange(parsedVersion, r, allParsedRanges, options)) {
                return true;
            }
        }
        return false;
    }
    else if (range.includes(' - ')) {
        // Hyphen ranges: https://github.com/npm/node-semver#hyphen-ranges-xyz---abc
        range = replaceHyphen(range, options);
    }
    else if (range.includes(' ')) {
        // Multiple separated ranges and all needs to be satisfied for success
        const ranges = range
            .trim()
            .replace(/\s{2,}/g, ' ')
            .split(' ');
        for (const r of ranges) {
            if (!_checkRange(parsedVersion, r, allParsedRanges, options)) {
                return false;
            }
        }
        return true;
    }
    // Check given parsed version with given range
    return _checkRange(parsedVersion, range, allParsedRanges, options);
}
function _checkRange(parsedVersion, range, allParsedRanges, options) {
    range = _normalizeRange(range, options);
    if (range.includes(' ')) {
        // If there are multiple ranges separated, satisfy each of them
        return _doSatisfies(parsedVersion, range, allParsedRanges, options);
    }
    else {
        // Validate and parse range
        const parsedRange = _parseRange(range);
        allParsedRanges.push(parsedRange);
        // Check parsed version by parsed range
        return _satisfies(parsedVersion, parsedRange);
    }
}
function _satisfies(parsedVersion, parsedRange) {
    // If range is invalid, satisfy check fails (no error throw)
    if (parsedRange.invalid) {
        return false;
    }
    // If range is empty or wildcard, satisfy check succeeds regardless what version is
    if (!parsedRange.version || _isWildcard(parsedRange.version)) {
        return true;
    }
    // Compare version segment first
    let comparisonResult = _compareVersionSegments(parsedVersion.versionSegments || [], parsedRange.versionSegments || []);
    // If versions segments are equal, compare by pre-release segments
    if (comparisonResult === 0) {
        const versionPrereleaseSegments = parsedVersion.prereleaseSegments || [];
        const rangePrereleaseSegments = parsedRange.prereleaseSegments || [];
        if (!versionPrereleaseSegments.length && !rangePrereleaseSegments.length) {
            comparisonResult = 0;
        }
        else if (!versionPrereleaseSegments.length &&
            rangePrereleaseSegments.length) {
            comparisonResult = 1;
        }
        else if (versionPrereleaseSegments.length &&
            !rangePrereleaseSegments.length) {
            comparisonResult = -1;
        }
        else {
            comparisonResult = _compareVersionSegments(versionPrereleaseSegments, rangePrereleaseSegments);
        }
    }
    // Resolve check result according to comparison operator
    return operatorResMap[parsedRange.op]?.includes(comparisonResult);
}
function _doPreleaseCheck(parsedVersion, allParsedRanges) {
    if (parsedVersion.prerelease) {
        return allParsedRanges.some(r => r.prerelease && r.version === parsedVersion.version);
    }
    return true;
}
function _normalizeRange(range, options) {
    range = range.trim();
    range = replaceCaret(range, options);
    range = replaceTilde(range);
    range = replaceXRange(range, options);
    range = range.trim();
    return range;
}
function isX(id) {
    return !id || id.toLowerCase() === 'x' || id === '*';
}
function _parseVersion(versionString) {
    const match = versionString.match(VERSION_REGEXP);
    if (!match) {
        diag_api_diag.error(`Invalid version: ${versionString}`);
        return undefined;
    }
    const version = match.groups.version;
    const prerelease = match.groups.prerelease;
    const build = match.groups.build;
    const versionSegments = version.split('.');
    const prereleaseSegments = prerelease?.split('.');
    return {
        op: undefined,
        version,
        versionSegments,
        versionSegmentCount: versionSegments.length,
        prerelease,
        prereleaseSegments,
        prereleaseSegmentCount: prereleaseSegments ? prereleaseSegments.length : 0,
        build,
    };
}
function _parseRange(rangeString) {
    if (!rangeString) {
        return {};
    }
    const match = rangeString.match(RANGE_REGEXP);
    if (!match) {
        diag_api_diag.error(`Invalid range: ${rangeString}`);
        return {
            invalid: true,
        };
    }
    let op = match.groups.op;
    const version = match.groups.version;
    const prerelease = match.groups.prerelease;
    const build = match.groups.build;
    const versionSegments = version.split('.');
    const prereleaseSegments = prerelease?.split('.');
    if (op === '==') {
        op = '=';
    }
    return {
        op: op || '=',
        version,
        versionSegments,
        versionSegmentCount: versionSegments.length,
        prerelease,
        prereleaseSegments,
        prereleaseSegmentCount: prereleaseSegments ? prereleaseSegments.length : 0,
        build,
    };
}
function _isWildcard(s) {
    return s === '*' || s === 'x' || s === 'X';
}
function _parseVersionString(v) {
    const n = parseInt(v, 10);
    return isNaN(n) ? v : n;
}
function _normalizeVersionType(a, b) {
    if (typeof a === typeof b) {
        if (typeof a === 'number') {
            return [a, b];
        }
        else if (typeof a === 'string') {
            return [a, b];
        }
        else {
            throw new Error('Version segments can only be strings or numbers');
        }
    }
    else {
        return [String(a), String(b)];
    }
}
function _compareVersionStrings(v1, v2) {
    if (_isWildcard(v1) || _isWildcard(v2)) {
        return 0;
    }
    const [parsedV1, parsedV2] = _normalizeVersionType(_parseVersionString(v1), _parseVersionString(v2));
    if (parsedV1 > parsedV2) {
        return 1;
    }
    else if (parsedV1 < parsedV2) {
        return -1;
    }
    return 0;
}
function _compareVersionSegments(v1, v2) {
    for (let i = 0; i < Math.max(v1.length, v2.length); i++) {
        const res = _compareVersionStrings(v1[i] || '0', v2[i] || '0');
        if (res !== 0) {
            return res;
        }
    }
    return 0;
}
////////////////////////////////////////////////////////////////////////////////
// The rest of this file is adapted from portions of https://github.com/npm/node-semver/tree/868d4bb
// License:
/*
 * The ISC License
 *
 * Copyright (c) Isaac Z. Schlueter and Contributors
 *
 * Permission to use, copy, modify, and/or distribute this software for any
 * purpose with or without fee is hereby granted, provided that the above
 * copyright notice and this permission notice appear in all copies.
 *
 * THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
 * WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
 * MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
 * ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
 * WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
 * ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF OR
 * IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.
 */
const LETTERDASHNUMBER = '[a-zA-Z0-9-]';
const NUMERICIDENTIFIER = '0|[1-9]\\d*';
const NONNUMERICIDENTIFIER = `\\d*[a-zA-Z-]${LETTERDASHNUMBER}*`;
const GTLT = '((?:<|>)?=?)';
const PRERELEASEIDENTIFIER = `(?:${NUMERICIDENTIFIER}|${NONNUMERICIDENTIFIER})`;
const PRERELEASE = `(?:-(${PRERELEASEIDENTIFIER}(?:\\.${PRERELEASEIDENTIFIER})*))`;
const BUILDIDENTIFIER = `${LETTERDASHNUMBER}+`;
const BUILD = `(?:\\+(${BUILDIDENTIFIER}(?:\\.${BUILDIDENTIFIER})*))`;
const XRANGEIDENTIFIER = `${NUMERICIDENTIFIER}|x|X|\\*`;
const XRANGEPLAIN = `[v=\\s]*(${XRANGEIDENTIFIER})` +
    `(?:\\.(${XRANGEIDENTIFIER})` +
    `(?:\\.(${XRANGEIDENTIFIER})` +
    `(?:${PRERELEASE})?${BUILD}?` +
    ')?)?';
const XRANGE = `^${GTLT}\\s*${XRANGEPLAIN}$`;
const XRANGE_REGEXP = new RegExp(XRANGE);
const HYPHENRANGE = `^\\s*(${XRANGEPLAIN})` + '\\s+-\\s+' + `(${XRANGEPLAIN})` + '\\s*$';
const HYPHENRANGE_REGEXP = new RegExp(HYPHENRANGE);
const LONETILDE = '(?:~>?)';
const TILDE = `^${LONETILDE}${XRANGEPLAIN}$`;
const TILDE_REGEXP = new RegExp(TILDE);
const LONECARET = '(?:\\^)';
const CARET = `^${LONECARET}${XRANGEPLAIN}$`;
const CARET_REGEXP = new RegExp(CARET);
// Borrowed from https://github.com/npm/node-semver/blob/868d4bbe3d318c52544f38d5f9977a1103e924c2/classes/range.js#L285
//
// ~, ~> --> * (any, kinda silly)
// ~2, ~2.x, ~2.x.x, ~>2, ~>2.x ~>2.x.x --> >=2.0.0 <3.0.0-0
// ~2.0, ~2.0.x, ~>2.0, ~>2.0.x --> >=2.0.0 <2.1.0-0
// ~1.2, ~1.2.x, ~>1.2, ~>1.2.x --> >=1.2.0 <1.3.0-0
// ~1.2.3, ~>1.2.3 --> >=1.2.3 <1.3.0-0
// ~1.2.0, ~>1.2.0 --> >=1.2.0 <1.3.0-0
// ~0.0.1 --> >=0.0.1 <0.1.0-0
function replaceTilde(comp) {
    const r = TILDE_REGEXP;
    return comp.replace(r, (_, M, m, p, pr) => {
        let ret;
        if (isX(M)) {
            ret = '';
        }
        else if (isX(m)) {
            ret = `>=${M}.0.0 <${+M + 1}.0.0-0`;
        }
        else if (isX(p)) {
            // ~1.2 == >=1.2.0 <1.3.0-0
            ret = `>=${M}.${m}.0 <${M}.${+m + 1}.0-0`;
        }
        else if (pr) {
            ret = `>=${M}.${m}.${p}-${pr} <${M}.${+m + 1}.0-0`;
        }
        else {
            // ~1.2.3 == >=1.2.3 <1.3.0-0
            ret = `>=${M}.${m}.${p} <${M}.${+m + 1}.0-0`;
        }
        return ret;
    });
}
// Borrowed from https://github.com/npm/node-semver/blob/868d4bbe3d318c52544f38d5f9977a1103e924c2/classes/range.js#L329
//
// ^ --> * (any, kinda silly)
// ^2, ^2.x, ^2.x.x --> >=2.0.0 <3.0.0-0
// ^2.0, ^2.0.x --> >=2.0.0 <3.0.0-0
// ^1.2, ^1.2.x --> >=1.2.0 <2.0.0-0
// ^1.2.3 --> >=1.2.3 <2.0.0-0
// ^1.2.0 --> >=1.2.0 <2.0.0-0
// ^0.0.1 --> >=0.0.1 <0.0.2-0
// ^0.1.0 --> >=0.1.0 <0.2.0-0
function replaceCaret(comp, options) {
    const r = CARET_REGEXP;
    const z = options?.includePrerelease ? '-0' : '';
    return comp.replace(r, (_, M, m, p, pr) => {
        let ret;
        if (isX(M)) {
            ret = '';
        }
        else if (isX(m)) {
            ret = `>=${M}.0.0${z} <${+M + 1}.0.0-0`;
        }
        else if (isX(p)) {
            if (M === '0') {
                ret = `>=${M}.${m}.0${z} <${M}.${+m + 1}.0-0`;
            }
            else {
                ret = `>=${M}.${m}.0${z} <${+M + 1}.0.0-0`;
            }
        }
        else if (pr) {
            if (M === '0') {
                if (m === '0') {
                    ret = `>=${M}.${m}.${p}-${pr} <${M}.${m}.${+p + 1}-0`;
                }
                else {
                    ret = `>=${M}.${m}.${p}-${pr} <${M}.${+m + 1}.0-0`;
                }
            }
            else {
                ret = `>=${M}.${m}.${p}-${pr} <${+M + 1}.0.0-0`;
            }
        }
        else {
            if (M === '0') {
                if (m === '0') {
                    ret = `>=${M}.${m}.${p}${z} <${M}.${m}.${+p + 1}-0`;
                }
                else {
                    ret = `>=${M}.${m}.${p}${z} <${M}.${+m + 1}.0-0`;
                }
            }
            else {
                ret = `>=${M}.${m}.${p} <${+M + 1}.0.0-0`;
            }
        }
        return ret;
    });
}
// Borrowed from https://github.com/npm/node-semver/blob/868d4bbe3d318c52544f38d5f9977a1103e924c2/classes/range.js#L390
function replaceXRange(comp, options) {
    const r = XRANGE_REGEXP;
    return comp.replace(r, (ret, gtlt, M, m, p, pr) => {
        const xM = isX(M);
        const xm = xM || isX(m);
        const xp = xm || isX(p);
        const anyX = xp;
        if (gtlt === '=' && anyX) {
            gtlt = '';
        }
        // if we're including prereleases in the match, then we need
        // to fix this to -0, the lowest possible prerelease value
        pr = options?.includePrerelease ? '-0' : '';
        if (xM) {
            if (gtlt === '>' || gtlt === '<') {
                // nothing is allowed
                ret = '<0.0.0-0';
            }
            else {
                // nothing is forbidden
                ret = '*';
            }
        }
        else if (gtlt && anyX) {
            // we know patch is an x, because we have any x at all.
            // replace X with 0
            if (xm) {
                m = 0;
            }
            p = 0;
            if (gtlt === '>') {
                // >1 => >=2.0.0
                // >1.2 => >=1.3.0
                gtlt = '>=';
                if (xm) {
                    M = +M + 1;
                    m = 0;
                    p = 0;
                }
                else {
                    m = +m + 1;
                    p = 0;
                }
            }
            else if (gtlt === '<=') {
                // <=0.7.x is actually <0.8.0, since any 0.7.x should
                // pass.  Similarly, <=7.x is actually <8.0.0, etc.
                gtlt = '<';
                if (xm) {
                    M = +M + 1;
                }
                else {
                    m = +m + 1;
                }
            }
            if (gtlt === '<') {
                pr = '-0';
            }
            ret = `${gtlt + M}.${m}.${p}${pr}`;
        }
        else if (xm) {
            ret = `>=${M}.0.0${pr} <${+M + 1}.0.0-0`;
        }
        else if (xp) {
            ret = `>=${M}.${m}.0${pr} <${M}.${+m + 1}.0-0`;
        }
        return ret;
    });
}
// Borrowed from https://github.com/npm/node-semver/blob/868d4bbe3d318c52544f38d5f9977a1103e924c2/classes/range.js#L488
//
// 1.2 - 3.4.5 => >=1.2.0 <=3.4.5
// 1.2.3 - 3.4 => >=1.2.0 <3.5.0-0 Any 3.4.x will do
// 1.2 - 3.4 => >=1.2.0 <3.5.0-0
function replaceHyphen(comp, options) {
    const r = HYPHENRANGE_REGEXP;
    return comp.replace(r, (_, from, fM, fm, fp, fpr, fb, to, tM, tm, tp, tpr) => {
        if (isX(fM)) {
            from = '';
        }
        else if (isX(fm)) {
            from = `>=${fM}.0.0${options?.includePrerelease ? '-0' : ''}`;
        }
        else if (isX(fp)) {
            from = `>=${fM}.${fm}.0${options?.includePrerelease ? '-0' : ''}`;
        }
        else if (fpr) {
            from = `>=${from}`;
        }
        else {
            from = `>=${from}${options?.includePrerelease ? '-0' : ''}`;
        }
        if (isX(tM)) {
            to = '';
        }
        else if (isX(tm)) {
            to = `<${+tM + 1}.0.0-0`;
        }
        else if (isX(tp)) {
            to = `<${tM}.${+tm + 1}.0-0`;
        }
        else if (tpr) {
            to = `<=${tM}.${tm}.${tp}-${tpr}`;
        }
        else if (options?.includePrerelease) {
            to = `<${tM}.${tm}.${+tp + 1}-0`;
        }
        else {
            to = `<=${to}`;
        }
        return `${from} ${to}`.trim();
    });
}
//# sourceMappingURL=semver.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+instrumentation@0.222.0+4294d048f15f6cd7/node_modules/@opentelemetry/instrumentation/build/esm/shimmer.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
// Default to complaining loudly when things don't go according to plan.
// eslint-disable-next-line no-console
let logger = console.error.bind(console);
// Sets a property on an object, preserving its enumerability.
// This function assumes that the property is already writable.
function defineProperty(obj, name, value) {
    const enumerable = !!obj[name] &&
        Object.prototype.propertyIsEnumerable.call(obj, name);
    Object.defineProperty(obj, name, {
        configurable: true,
        enumerable,
        writable: true,
        value,
    });
}
const wrap = (nodule, name, wrapper) => {
    if (!nodule || !nodule[name]) {
        logger('no original function ' + String(name) + ' to wrap');
        return;
    }
    if (!wrapper) {
        logger('no wrapper function');
        logger(new Error().stack);
        return;
    }
    const original = nodule[name];
    if (typeof original !== 'function' || typeof wrapper !== 'function') {
        logger('original object and wrapper must be functions');
        return;
    }
    const wrapped = wrapper(original, name);
    defineProperty(wrapped, '__original', original);
    defineProperty(wrapped, '__unwrap', () => {
        if (nodule[name] === wrapped) {
            defineProperty(nodule, name, original);
        }
    });
    defineProperty(wrapped, '__wrapped', true);
    defineProperty(nodule, name, wrapped);
    return wrapped;
};
const massWrap = (nodules, names, wrapper) => {
    if (!nodules) {
        logger('must provide one or more modules to patch');
        logger(new Error().stack);
        return;
    }
    else if (!Array.isArray(nodules)) {
        nodules = [nodules];
    }
    if (!(names && Array.isArray(names))) {
        logger('must provide one or more functions to wrap on modules');
        return;
    }
    nodules.forEach(nodule => {
        names.forEach(name => {
            wrap(nodule, name, wrapper);
        });
    });
};
const unwrap = (nodule, name) => {
    if (!nodule || !nodule[name]) {
        logger('no function to unwrap.');
        logger(new Error().stack);
        return;
    }
    const wrapped = nodule[name];
    if (!wrapped.__unwrap) {
        logger('no original to unwrap to -- has ' +
            String(name) +
            ' already been unwrapped?');
    }
    else {
        wrapped.__unwrap();
        return;
    }
};
const massUnwrap = (nodules, names) => {
    if (!nodules) {
        logger('must provide one or more modules to patch');
        logger(new Error().stack);
        return;
    }
    else if (!Array.isArray(nodules)) {
        nodules = [nodules];
    }
    if (!(names && Array.isArray(names))) {
        logger('must provide one or more functions to unwrap on modules');
        return;
    }
    nodules.forEach(nodule => {
        names.forEach(name => {
            unwrap(nodule, name);
        });
    });
};
function shimmer(options) {
    if (options && options.logger) {
        if (typeof options.logger !== 'function') {
            logger("new logger isn't a function, not replacing");
        }
        else {
            logger = options.logger;
        }
    }
}
shimmer.wrap = wrap;
shimmer.massWrap = massWrap;
shimmer.unwrap = unwrap;
shimmer.massUnwrap = massUnwrap;
//# sourceMappingURL=shimmer.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+instrumentation@0.222.0+4294d048f15f6cd7/node_modules/@opentelemetry/instrumentation/build/esm/instrumentation.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */



/**
 * Base abstract internal class for instrumenting node and web plugins
 */
class InstrumentationAbstract {
    _config = {};
    _tracer;
    _meter;
    _logger;
    _diag;
    instrumentationName;
    instrumentationVersion;
    constructor(instrumentationName, instrumentationVersion, config) {
        this.instrumentationName = instrumentationName;
        this.instrumentationVersion = instrumentationVersion;
        this.setConfig(config);
        this._diag = diag_api_diag.createComponentLogger({
            namespace: instrumentationName,
        });
        this._tracer = trace_api/* trace */.u.getTracer(instrumentationName, instrumentationVersion);
        this._meter = metrics.getMeter(instrumentationName, instrumentationVersion);
        this._logger = logs.getLogger(instrumentationName, instrumentationVersion);
        this._updateMetricInstruments();
    }
    /* Api to wrap instrumented method */
    _wrap = wrap;
    /* Api to unwrap instrumented methods */
    _unwrap = unwrap;
    /* Api to mass wrap instrumented method */
    _massWrap = massWrap;
    /* Api to mass unwrap instrumented methods */
    _massUnwrap = massUnwrap;
    /* Returns meter */
    get meter() {
        return this._meter;
    }
    /**
     * Sets MeterProvider to this plugin
     * @param meterProvider
     */
    setMeterProvider(meterProvider) {
        this._meter = meterProvider.getMeter(this.instrumentationName, this.instrumentationVersion);
        this._updateMetricInstruments();
    }
    /* Returns logger */
    get logger() {
        return this._logger;
    }
    /**
     * Sets LoggerProvider to this plugin
     * @param loggerProvider
     */
    setLoggerProvider(loggerProvider) {
        this._logger = loggerProvider.getLogger(this.instrumentationName, this.instrumentationVersion);
    }
    /**
     * @experimental
     *
     * Get module definitions defined by {@link init}.
     * This can be used for experimental compile-time instrumentation.
     *
     * @returns an array of {@link InstrumentationModuleDefinition}
     */
    getModuleDefinitions() {
        const initResult = this.init() ?? [];
        if (!Array.isArray(initResult)) {
            return [initResult];
        }
        return initResult;
    }
    /**
     * Sets the new metric instruments with the current Meter.
     */
    _updateMetricInstruments() {
        return;
    }
    /* Returns InstrumentationConfig */
    getConfig() {
        return this._config;
    }
    /**
     * Sets InstrumentationConfig to this plugin
     * @param config
     */
    setConfig(config) {
        // copy config first level properties to ensure they are immutable.
        // nested properties are not copied, thus are mutable from the outside.
        this._config = {
            enabled: true,
            ...config,
        };
    }
    /**
     * Sets TracerProvider to this plugin
     * @param tracerProvider
     */
    setTracerProvider(tracerProvider) {
        this._tracer = tracerProvider.getTracer(this.instrumentationName, this.instrumentationVersion);
    }
    /* Returns tracer */
    get tracer() {
        return this._tracer;
    }
    /**
     * Execute span customization hook, if configured, and log any errors.
     * Any semantics of the trigger and info are defined by the specific instrumentation.
     * @param hookHandler The optional hook handler which the user has configured via instrumentation config
     * @param triggerName The name of the trigger for executing the hook for logging purposes
     * @param span The span to which the hook should be applied
     * @param info The info object to be passed to the hook, with useful data the hook may use
     */
    _runSpanCustomizationHook(hookHandler, triggerName, span, info) {
        if (!hookHandler) {
            return;
        }
        try {
            hookHandler(span, info);
        }
        catch (e) {
            this._diag.error('Error running span customization hook due to exception in handler', { triggerName }, e);
        }
    }
}
//# sourceMappingURL=instrumentation.js.map
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/require-in-the-middle@8.0.1+759ce506b1ed1a42/node_modules/require-in-the-middle/index.js
var require_in_the_middle = __webpack_require__(78159);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+instrumentation@0.222.0+4294d048f15f6cd7/node_modules/@opentelemetry/instrumentation/build/esm/platform/node/ModuleNameTrie.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
const ModuleNameSeparator = '/';
/**
 * Node in a `ModuleNameTrie`
 */
class ModuleNameTrieNode {
    hooks = [];
    children = new Map();
}
/**
 * Trie containing nodes that represent a part of a module name (i.e. the parts separated by forward slash)
 */
class ModuleNameTrie {
    _trie = new ModuleNameTrieNode();
    _counter = 0;
    /**
     * Insert a module hook into the trie
     *
     * @param {Hooked} hook Hook
     */
    insert(hook) {
        let trieNode = this._trie;
        for (const moduleNamePart of hook.moduleName.split(ModuleNameSeparator)) {
            let nextNode = trieNode.children.get(moduleNamePart);
            if (!nextNode) {
                nextNode = new ModuleNameTrieNode();
                trieNode.children.set(moduleNamePart, nextNode);
            }
            trieNode = nextNode;
        }
        trieNode.hooks.push({ hook, insertedId: this._counter++ });
    }
    /**
     * Search for matching hooks in the trie
     *
     * @param {string} moduleName Module name
     * @param {boolean} maintainInsertionOrder Whether to return the results in insertion order
     * @param {boolean} fullOnly Whether to return only full matches
     * @returns {Hooked[]} Matching hooks
     */
    search(moduleName, { maintainInsertionOrder, fullOnly } = {}) {
        let trieNode = this._trie;
        const results = [];
        let foundFull = true;
        for (const moduleNamePart of moduleName.split(ModuleNameSeparator)) {
            const nextNode = trieNode.children.get(moduleNamePart);
            if (!nextNode) {
                foundFull = false;
                break;
            }
            if (!fullOnly) {
                results.push(...nextNode.hooks);
            }
            trieNode = nextNode;
        }
        if (fullOnly && foundFull) {
            results.push(...trieNode.hooks);
        }
        if (results.length === 0) {
            return [];
        }
        if (results.length === 1) {
            return [results[0].hook];
        }
        if (maintainInsertionOrder) {
            results.sort((a, b) => a.insertedId - b.insertedId);
        }
        return results.map(({ hook }) => hook);
    }
}
//# sourceMappingURL=ModuleNameTrie.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+instrumentation@0.222.0+4294d048f15f6cd7/node_modules/@opentelemetry/instrumentation/build/esm/platform/node/RequireInTheMiddleSingleton.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */



/**
 * Whether Mocha is running in this process
 * Inspired by https://github.com/AndreasPizsa/detect-mocha
 *
 * @type {boolean}
 */
const isMocha = [
    'afterEach',
    'after',
    'beforeEach',
    'before',
    'describe',
    'it',
].every(fn => {
    // @ts-expect-error TS7053: Element implicitly has an 'any' type
    return typeof global[fn] === 'function';
});
/**
 * Singleton class for `require-in-the-middle`
 * Allows instrumentation plugins to patch modules with only a single `require` patch
 * WARNING: Because this class will create its own `require-in-the-middle` (RITM) instance,
 * we should minimize the number of new instances of this class.
 * Multiple instances of `@opentelemetry/instrumentation` (e.g. multiple versions) in a single process
 * will result in multiple instances of RITM, which will have an impact
 * on the performance of instrumentation hooks being applied.
 */
class RequireInTheMiddleSingleton {
    _moduleNameTrie = new ModuleNameTrie();
    static _instance;
    constructor() {
        this._initialize();
    }
    _initialize() {
        new require_in_the_middle.Hook(
        // Intercept all `require` calls; we will filter the matching ones below
        null, { internals: true }, (exports, name, basedir) => {
            // For internal files on Windows, `name` will use backslash as the path separator
            const normalizedModuleName = normalizePathSeparators(name);
            const matches = this._moduleNameTrie.search(normalizedModuleName, {
                maintainInsertionOrder: true,
                // For core modules (e.g. `fs`), do not match on sub-paths (e.g. `fs/promises').
                // This matches the behavior of `require-in-the-middle`.
                // `basedir` is always `undefined` for core modules.
                fullOnly: basedir === undefined,
            });
            for (const { onRequire } of matches) {
                exports = onRequire(exports, name, basedir);
            }
            return exports;
        });
    }
    /**
     * Register a hook with `require-in-the-middle`
     *
     * @param {string} moduleName Module name
     * @param {OnRequireFn} onRequire Hook function
     * @returns {Hooked} Registered hook
     */
    register(moduleName, onRequire) {
        const hooked = { moduleName, onRequire };
        this._moduleNameTrie.insert(hooked);
        return hooked;
    }
    /**
     * Get the `RequireInTheMiddleSingleton` singleton
     *
     * @returns {RequireInTheMiddleSingleton} Singleton of `RequireInTheMiddleSingleton`
     */
    static getInstance() {
        // Mocha runs all test suites in the same process
        // This prevents test suites from sharing a singleton
        if (isMocha)
            return new RequireInTheMiddleSingleton();
        return (this._instance =
            this._instance ?? new RequireInTheMiddleSingleton());
    }
}
/**
 * Normalize the path separators to forward slash in a module name or path
 *
 * @param {string} moduleNameOrPath Module name or path
 * @returns {string} Normalized module name or path
 */
function normalizePathSeparators(moduleNameOrPath) {
    return external_path_.sep !== ModuleNameSeparator
        ? moduleNameOrPath.split(external_path_.sep).join(ModuleNameSeparator)
        : moduleNameOrPath;
}
//# sourceMappingURL=RequireInTheMiddleSingleton.js.map
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/import-in-the-middle@3.4.0/node_modules/import-in-the-middle/index.js
var import_in_the_middle = __webpack_require__(64340);
// EXTERNAL MODULE: external "fs"
var external_fs_ = __webpack_require__(29021);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+instrumentation@0.222.0+4294d048f15f6cd7/node_modules/@opentelemetry/instrumentation/build/esm/utils.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
/**
 * function to execute patched function and being able to catch errors
 * @param execute - function to be executed
 * @param onFinish - callback to run when execute finishes
 */
function safeExecuteInTheMiddle(execute, onFinish, preventThrowingError) {
    let error;
    let result;
    try {
        result = execute();
    }
    catch (e) {
        error = e;
    }
    finally {
        onFinish(error, result);
        if (error && !preventThrowingError) {
            // eslint-disable-next-line no-unsafe-finally
            throw error;
        }
        // eslint-disable-next-line no-unsafe-finally
        return result;
    }
}
/**
 * Async function to execute patched function and being able to catch errors
 * @param execute - function to be executed
 * @param onFinish - callback to run when execute finishes
 */
async function safeExecuteInTheMiddleAsync(execute, onFinish, preventThrowingError) {
    let error;
    let result;
    try {
        result = await execute();
    }
    catch (e) {
        error = e;
    }
    finally {
        await onFinish(error, result);
        if (error && !preventThrowingError) {
            // eslint-disable-next-line no-unsafe-finally
            throw error;
        }
        // eslint-disable-next-line no-unsafe-finally
        return result;
    }
}
/**
 * Checks if certain function has been already wrapped
 * @param func
 */
function isWrapped(func) {
    return (typeof func === 'function' &&
        typeof func.__original === 'function' &&
        typeof func.__unwrap === 'function' &&
        func.__wrapped === true);
}
//# sourceMappingURL=utils.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+instrumentation@0.222.0+4294d048f15f6cd7/node_modules/@opentelemetry/instrumentation/build/esm/platform/node/instrumentation.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */











/**
 * Base abstract class for instrumenting node plugins
 */
class InstrumentationBase extends InstrumentationAbstract {
    _modules;
    _hooks = [];
    _requireInTheMiddleSingleton = RequireInTheMiddleSingleton.getInstance();
    _enabled = false;
    constructor(instrumentationName, instrumentationVersion, config) {
        super(instrumentationName, instrumentationVersion, config);
        let modules = this.init();
        if (modules && !Array.isArray(modules)) {
            modules = [modules];
        }
        this._modules = modules || [];
        if (this._config.enabled) {
            this.enable();
        }
    }
    _wrap = (moduleExports, name, wrapper) => {
        if (isWrapped(moduleExports[name])) {
            this._unwrap(moduleExports, name);
        }
        if (!external_util_.types.isProxy(moduleExports)) {
            return wrap(moduleExports, name, wrapper);
        }
        else {
            const wrapped = wrap(Object.assign({}, moduleExports), name, wrapper);
            Object.defineProperty(moduleExports, name, {
                value: wrapped,
            });
            return wrapped;
        }
    };
    _unwrap = (moduleExports, name) => {
        if (!external_util_.types.isProxy(moduleExports)) {
            return unwrap(moduleExports, name);
        }
        else {
            return Object.defineProperty(moduleExports, name, {
                value: moduleExports[name],
            });
        }
    };
    _massWrap = (moduleExportsArray, names, wrapper) => {
        if (!moduleExportsArray) {
            diag_api_diag.error('must provide one or more modules to patch');
            return;
        }
        else if (!Array.isArray(moduleExportsArray)) {
            moduleExportsArray = [moduleExportsArray];
        }
        if (!(names && Array.isArray(names))) {
            diag_api_diag.error('must provide one or more functions to wrap on modules');
            return;
        }
        moduleExportsArray.forEach(moduleExports => {
            names.forEach(name => {
                this._wrap(moduleExports, name, wrapper);
            });
        });
    };
    _massUnwrap = (moduleExportsArray, names) => {
        if (!moduleExportsArray) {
            diag_api_diag.error('must provide one or more modules to patch');
            return;
        }
        else if (!Array.isArray(moduleExportsArray)) {
            moduleExportsArray = [moduleExportsArray];
        }
        if (!(names && Array.isArray(names))) {
            diag_api_diag.error('must provide one or more functions to wrap on modules');
            return;
        }
        moduleExportsArray.forEach(moduleExports => {
            names.forEach(name => {
                this._unwrap(moduleExports, name);
            });
        });
    };
    _warnOnPreloadedModules() {
        // Access require via globalThis to prevent webpack from analyzing it as a dependency expression
        const nodeRequire = globalThis.require;
        if (!nodeRequire?.resolve || !nodeRequire?.cache)
            return;
        this._modules.forEach((module) => {
            const { name } = module;
            try {
                const resolvedModule = nodeRequire.resolve(name);
                if (nodeRequire.cache[resolvedModule]?.loaded) {
                    // Module is already cached, which means the instrumentation hook might not work
                    this._diag.warn(`Module ${name} has been loaded before ${this.instrumentationName} so it might not work, please initialize it before requiring ${name}`);
                }
            }
            catch {
                // Module isn't available, we can simply skip
            }
        });
    }
    _extractPackageVersion(baseDir) {
        try {
            const json = (0,external_fs_.readFileSync)(external_path_.join(baseDir, 'package.json'), {
                encoding: 'utf8',
            });
            const version = JSON.parse(json).version;
            return typeof version === 'string' ? version : undefined;
        }
        catch {
            diag_api_diag.warn('Failed extracting version', baseDir);
        }
        return undefined;
    }
    _onRequire(module, exports, name, baseDir) {
        if (!baseDir) {
            if (typeof module.patch === 'function') {
                module.moduleExports = exports;
                if (this._enabled) {
                    this._diag.debug('Applying instrumentation patch for nodejs core module on require hook', {
                        module: module.name,
                    });
                    return module.patch(exports);
                }
            }
            return exports;
        }
        const version = this._extractPackageVersion(baseDir);
        module.moduleVersion = version;
        if (module.name === name) {
            // main module
            if (isSupported(module.supportedVersions, version, module.includePrerelease)) {
                if (typeof module.patch === 'function') {
                    module.moduleExports = exports;
                    if (this._enabled) {
                        this._diag.debug('Applying instrumentation patch for module on require hook', {
                            module: module.name,
                            version: module.moduleVersion,
                            baseDir,
                        });
                        return module.patch(exports, module.moduleVersion);
                    }
                }
            }
            return exports;
        }
        // internal file
        const files = module.files ?? [];
        const normalizedName = external_path_.normalize(name);
        const supportedFileInstrumentations = files.filter(f => f.name === normalizedName &&
            isSupported(f.supportedVersions, version, module.includePrerelease));
        return supportedFileInstrumentations.reduce((patchedExports, file) => {
            file.moduleExports = patchedExports;
            if (this._enabled) {
                this._diag.debug('Applying instrumentation patch for nodejs module file on require hook', {
                    module: module.name,
                    version: module.moduleVersion,
                    fileName: file.name,
                    baseDir,
                });
                // patch signature is not typed, so we cast it assuming it's correct
                return file.patch(patchedExports, module.moduleVersion);
            }
            return patchedExports;
        }, exports);
    }
    enable() {
        if (this._enabled) {
            return;
        }
        this._enabled = true;
        // already hooked, just call patch again
        if (this._hooks.length > 0) {
            for (const module of this._modules) {
                if (typeof module.patch === 'function' && module.moduleExports) {
                    this._diag.debug('Applying instrumentation patch for nodejs module on instrumentation enabled', {
                        module: module.name,
                        version: module.moduleVersion,
                    });
                    module.patch(module.moduleExports, module.moduleVersion);
                }
                for (const file of module.files) {
                    if (file.moduleExports) {
                        this._diag.debug('Applying instrumentation patch for nodejs module file on instrumentation enabled', {
                            module: module.name,
                            version: module.moduleVersion,
                            fileName: file.name,
                        });
                        file.patch(file.moduleExports, module.moduleVersion);
                    }
                }
            }
            return;
        }
        this._warnOnPreloadedModules();
        for (const module of this._modules) {
            const hookFn = (exports, name, baseDir) => {
                if (!baseDir && external_path_.isAbsolute(name)) {
                    // Change IITM `name` and `baseDir` values to match what RITM returns.
                    // See "Comparing to RITM" on https://github.com/nodejs/import-in-the-middle/pull/241
                    // for an example of the differences.
                    const parsedPath = external_path_.parse(name);
                    name = parsedPath.name;
                    baseDir = parsedPath.dir;
                }
                return this._onRequire(module, exports, name, baseDir);
            };
            const onRequire = (exports, name, baseDir) => {
                return this._onRequire(module, exports, name, baseDir);
            };
            // `RequireInTheMiddleSingleton` does not support absolute paths.
            // For an absolute paths, we must create a separate instance of the
            // require-in-the-middle `Hook`.
            const hook = external_path_.isAbsolute(module.name)
                ? new require_in_the_middle.Hook([module.name], { internals: true }, onRequire)
                : this._requireInTheMiddleSingleton.register(module.name, onRequire);
            this._hooks.push(hook);
            const esmHook = new import_in_the_middle.Hook([module.name], { internals: true }, hookFn);
            this._hooks.push(esmHook);
        }
    }
    disable() {
        if (!this._enabled) {
            return;
        }
        this._enabled = false;
        for (const module of this._modules) {
            if (typeof module.unpatch === 'function' && module.moduleExports) {
                this._diag.debug('Removing instrumentation patch for nodejs module on instrumentation disabled', {
                    module: module.name,
                    version: module.moduleVersion,
                });
                module.unpatch(module.moduleExports, module.moduleVersion);
            }
            for (const file of module.files) {
                if (file.moduleExports) {
                    this._diag.debug('Removing instrumentation patch for nodejs module file on instrumentation disabled', {
                        module: module.name,
                        version: module.moduleVersion,
                        fileName: file.name,
                    });
                    file.unpatch(file.moduleExports, module.moduleVersion);
                }
            }
        }
    }
    isEnabled() {
        return this._enabled;
    }
}
function isSupported(supportedVersions, version, includePrerelease) {
    if (typeof version === 'undefined') {
        // If we don't have the version, accept the wildcard case only
        return supportedVersions.includes('*');
    }
    return supportedVersions.some(supportedVersion => {
        return satisfies(version, supportedVersion, { includePrerelease });
    });
}
//# sourceMappingURL=instrumentation.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+semantic-conventions@1.43.0/node_modules/@opentelemetry/semantic-conventions/build/esm/stable_attributes.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
//----------------------------------------------------------------------------------------------------------
// DO NOT EDIT, this is an Auto-generated file from scripts/semconv/templates/registry/stable/attributes.ts.j2
//----------------------------------------------------------------------------------------------------------
/**
 * ASP.NET Core exception middleware handling result.
 *
 * @example handled
 * @example unhandled
 */
const ATTR_ASPNETCORE_DIAGNOSTICS_EXCEPTION_RESULT = 'aspnetcore.diagnostics.exception.result';
/**
 * Enum value "aborted" for attribute {@link ATTR_ASPNETCORE_DIAGNOSTICS_EXCEPTION_RESULT}.
 *
 * Exception handling didn't run because the request was aborted.
 */
const ASPNETCORE_DIAGNOSTICS_EXCEPTION_RESULT_VALUE_ABORTED = "aborted";
/**
 * Enum value "handled" for attribute {@link ATTR_ASPNETCORE_DIAGNOSTICS_EXCEPTION_RESULT}.
 *
 * Exception was handled by the exception handling middleware.
 */
const ASPNETCORE_DIAGNOSTICS_EXCEPTION_RESULT_VALUE_HANDLED = "handled";
/**
 * Enum value "skipped" for attribute {@link ATTR_ASPNETCORE_DIAGNOSTICS_EXCEPTION_RESULT}.
 *
 * Exception handling was skipped because the response had started.
 */
const ASPNETCORE_DIAGNOSTICS_EXCEPTION_RESULT_VALUE_SKIPPED = "skipped";
/**
 * Enum value "unhandled" for attribute {@link ATTR_ASPNETCORE_DIAGNOSTICS_EXCEPTION_RESULT}.
 *
 * Exception was not handled by the exception handling middleware.
 */
const ASPNETCORE_DIAGNOSTICS_EXCEPTION_RESULT_VALUE_UNHANDLED = "unhandled";
/**
 * Full type name of the [`IExceptionHandler`](https://learn.microsoft.com/dotnet/api/microsoft.aspnetcore.diagnostics.iexceptionhandler) implementation that handled the exception.
 *
 * @example Contoso.MyHandler
 */
const ATTR_ASPNETCORE_DIAGNOSTICS_HANDLER_TYPE = 'aspnetcore.diagnostics.handler.type';
/**
 * Rate limiting policy name.
 *
 * @example fixed
 * @example sliding
 * @example token
 */
const ATTR_ASPNETCORE_RATE_LIMITING_POLICY = 'aspnetcore.rate_limiting.policy';
/**
 * Rate-limiting result, shows whether the lease was acquired or contains a rejection reason
 *
 * @example acquired
 * @example request_canceled
 */
const ATTR_ASPNETCORE_RATE_LIMITING_RESULT = 'aspnetcore.rate_limiting.result';
/**
 * Enum value "acquired" for attribute {@link ATTR_ASPNETCORE_RATE_LIMITING_RESULT}.
 *
 * Lease was acquired
 */
const ASPNETCORE_RATE_LIMITING_RESULT_VALUE_ACQUIRED = "acquired";
/**
 * Enum value "endpoint_limiter" for attribute {@link ATTR_ASPNETCORE_RATE_LIMITING_RESULT}.
 *
 * Lease request was rejected by the endpoint limiter
 */
const ASPNETCORE_RATE_LIMITING_RESULT_VALUE_ENDPOINT_LIMITER = "endpoint_limiter";
/**
 * Enum value "global_limiter" for attribute {@link ATTR_ASPNETCORE_RATE_LIMITING_RESULT}.
 *
 * Lease request was rejected by the global limiter
 */
const ASPNETCORE_RATE_LIMITING_RESULT_VALUE_GLOBAL_LIMITER = "global_limiter";
/**
 * Enum value "request_canceled" for attribute {@link ATTR_ASPNETCORE_RATE_LIMITING_RESULT}.
 *
 * Lease request was canceled
 */
const ASPNETCORE_RATE_LIMITING_RESULT_VALUE_REQUEST_CANCELED = "request_canceled";
/**
 * Flag indicating if request was handled by the application pipeline.
 *
 * @example true
 */
const ATTR_ASPNETCORE_REQUEST_IS_UNHANDLED = 'aspnetcore.request.is_unhandled';
/**
 * A value that indicates whether the matched route is a fallback route.
 *
 * @example true
 */
const ATTR_ASPNETCORE_ROUTING_IS_FALLBACK = 'aspnetcore.routing.is_fallback';
/**
 * Match result - success or failure
 *
 * @example success
 * @example failure
 */
const ATTR_ASPNETCORE_ROUTING_MATCH_STATUS = 'aspnetcore.routing.match_status';
/**
 * Enum value "failure" for attribute {@link ATTR_ASPNETCORE_ROUTING_MATCH_STATUS}.
 *
 * Match failed
 */
const ASPNETCORE_ROUTING_MATCH_STATUS_VALUE_FAILURE = "failure";
/**
 * Enum value "success" for attribute {@link ATTR_ASPNETCORE_ROUTING_MATCH_STATUS}.
 *
 * Match succeeded
 */
const ASPNETCORE_ROUTING_MATCH_STATUS_VALUE_SUCCESS = "success";
/**
 * A value that indicates whether the user is authenticated.
 *
 * @example true
 */
const ATTR_ASPNETCORE_USER_IS_AUTHENTICATED = 'aspnetcore.user.is_authenticated';
/**
 * Client address - domain name if available without reverse DNS lookup; otherwise, IP address or Unix domain socket name.
 *
 * @example client.example.com
 * @example 10.1.2.80
 * @example /tmp/my.sock
 *
 * @note When observed from the server side, and when communicating through an intermediary, `client.address` **SHOULD** represent the client address behind any intermediaries,  for example proxies, if it's available.
 */
const ATTR_CLIENT_ADDRESS = 'client.address';
/**
 * Client port number.
 *
 * @example 65123
 *
 * @note When observed from the server side, and when communicating through an intermediary, `client.port` **SHOULD** represent the client port behind any intermediaries,  for example proxies, if it's available.
 */
const ATTR_CLIENT_PORT = 'client.port';
/**
 * The column number in `code.file.path` best representing the operation. It **SHOULD** point within the code unit named in `code.function.name`. This attribute **MUST NOT** be used on the Profile signal since the data is already captured in 'message Line'. This constraint is imposed to prevent redundancy and maintain data integrity.
 *
 * @example 16
 */
const ATTR_CODE_COLUMN_NUMBER = 'code.column.number';
/**
 * The source code file name that identifies the code unit as uniquely as possible (preferably an absolute file path). This attribute **MUST NOT** be used on the Profile signal since the data is already captured in 'message Function'. This constraint is imposed to prevent redundancy and maintain data integrity.
 *
 * @example "/usr/local/MyApplication/content_root/app/index.php"
 */
const ATTR_CODE_FILE_PATH = 'code.file.path';
/**
 * The method or function fully-qualified name without arguments. The value should fit the natural representation of the language runtime, which is also likely the same used within `code.stacktrace` attribute value. This attribute **MUST NOT** be used on the Profile signal since the data is already captured in 'message Function'. This constraint is imposed to prevent redundancy and maintain data integrity.
 *
 * @example com.example.MyHttpService.serveRequest
 * @example GuzzleHttp\\Client::transfer
 * @example fopen
 *
 * @note Values and format depends on each language runtime, thus it is impossible to provide an exhaustive list of examples.
 * The values are usually the same (or prefixes of) the ones found in native stack trace representation stored in
 * `code.stacktrace` without information on arguments.
 *
 * Examples:
 *
 *   - Java method: `com.example.MyHttpService.serveRequest`
 *   - Java anonymous class method: `com.mycompany.Main$1.myMethod`
 *   - Java lambda method: `com.mycompany.Main$$Lambda/0x0000748ae4149c00.myMethod`
 *   - PHP function: `GuzzleHttp\Client::transfer`
 *   - Go function: `github.com/my/repo/pkg.foo.func5`
 *   - Elixir: `OpenTelemetry.Ctx.new`
 *   - Erlang: `opentelemetry_ctx:new`
 *   - Rust: `playground::my_module::my_cool_func`
 *   - C function: `fopen`
 */
const ATTR_CODE_FUNCTION_NAME = 'code.function.name';
/**
 * The line number in `code.file.path` best representing the operation. It **SHOULD** point within the code unit named in `code.function.name`. This attribute **MUST NOT** be used on the Profile signal since the data is already captured in 'message Line'. This constraint is imposed to prevent redundancy and maintain data integrity.
 *
 * @example 42
 */
const ATTR_CODE_LINE_NUMBER = 'code.line.number';
/**
 * A stacktrace as a string in the natural representation for the language runtime. The representation is identical to [`exception.stacktrace`](/docs/exceptions/exceptions-spans.md#stacktrace-representation). This attribute **MUST NOT** be used on the Profile signal since the data is already captured in 'message Location'. This constraint is imposed to prevent redundancy and maintain data integrity.
 *
 * @example "at com.example.GenerateTrace.methodB(GenerateTrace.java:13)\\n at com.example.GenerateTrace.methodA(GenerateTrace.java:9)\\n at com.example.GenerateTrace.main(GenerateTrace.java:5)\\n"
 */
const ATTR_CODE_STACKTRACE = 'code.stacktrace';
/**
 * Container ID. Usually a UUID, as for example used to [identify Docker containers](https://docs.docker.com/engine/containers/run/#container-identification). The UUID might be abbreviated.
 *
 * @example a3bf90e006b2
 */
const ATTR_CONTAINER_ID = 'container.id';
/**
 * Name of the image the container was built on.
 *
 * @example gcr.io/opentelemetry/operator
 */
const ATTR_CONTAINER_IMAGE_NAME = 'container.image.name';
/**
 * Repo digests of the container image as provided by the container runtime.
 *
 * @example ["example@sha256:afcc7f1ac1b49db317a7196c902e61c6c3c4607d63599ee1a82d702d249a0ccb", "internal.registry.example.com:5000/example@sha256:b69959407d21e8a062e0416bf13405bb2b71ed7a84dde4158ebafacfa06f5578"]
 *
 * @note [Docker](https://docs.docker.com/reference/api/engine/version/v1.52/#tag/Image/operation/ImageInspect) and [CRI](https://github.com/kubernetes/cri-api/blob/c75ef5b473bbe2d0a4fc92f82235efd665ea8e9f/pkg/apis/runtime/v1/api.proto#L1237-L1238) report those under the `RepoDigests` field.
 */
const ATTR_CONTAINER_IMAGE_REPO_DIGESTS = 'container.image.repo_digests';
/**
 * Container image tags. An example can be found in [Docker Image Inspect](https://docs.docker.com/reference/api/engine/version/v1.52/#tag/Image/operation/ImageInspect). Should be only the `<tag>` section of the full name for example from `registry.example.com/my-org/my-image:<tag>`.
 *
 * @example ["v1.27.1", "3.5.7-0"]
 */
const ATTR_CONTAINER_IMAGE_TAGS = 'container.image.tags';
/**
 * The name of a collection (table, container) within the database.
 *
 * @example public.users
 * @example customers
 *
 * @note It is **RECOMMENDED** to capture the value as provided by the application
 * without attempting to do any case normalization.
 *
 * The collection name **SHOULD NOT** be extracted from `db.query.text`,
 * when the database system supports query text with multiple collections
 * in non-batch operations.
 *
 * For batch operations, if the individual operations are known to have the same
 * collection name then that collection name **SHOULD** be used.
 */
const ATTR_DB_COLLECTION_NAME = 'db.collection.name';
/**
 * The name of the database, fully qualified within the server address and port.
 *
 * @example customers
 * @example test.users
 *
 * @note If a database system has multiple namespace components, they **SHOULD** be concatenated from the most general to the most specific namespace component, using `|` as a separator between the components. Any missing components (and their associated separators) **SHOULD** be omitted.
 * Semantic conventions for individual database systems **SHOULD** document what `db.namespace` means in the context of that system.
 * It is **RECOMMENDED** to capture the value as provided by the application without attempting to do any case normalization.
 */
const ATTR_DB_NAMESPACE = 'db.namespace';
/**
 * The number of database operations included in a batch operation.
 *
 * @example 2
 * @example 3
 * @example 4
 *
 * @note Except for empty batch requests described below, a batch operation contains two
 * or more database operations explicitly submitted as separate operations in a single
 * client call, protocol message, or database command.
 *
 * Requests to batch APIs that contain only one operation **SHOULD** be modeled as single
 * operations, not as batch operations.
 *
 * A database call is not a batch operation solely because one operation accepts
 * multiple operands, such as keys, rows, documents, points, or other data elements,
 * including Redis [`MGET`](https://redis.io/docs/latest/commands/mget/) with
 * multiple keys.
 *
 * In batch APIs that execute the same parameterized operation with parameter sets,
 * each parameter set represents one database operation for determining whether the
 * request is a batch operation. Requests with only one parameter set **SHOULD** be modeled
 * as single operations, not as batch operations.
 *
 * `db.operation.batch.size` **SHOULD** be set to the number of operations in the batch.
 * It **SHOULD NOT** be set for non-batch operations.
 *
 * A request to execute a batch operation with no operations **SHOULD** also be treated
 * as a batch operation, and `db.operation.batch.size` **SHOULD** be set to `0`.
 */
const ATTR_DB_OPERATION_BATCH_SIZE = 'db.operation.batch.size';
/**
 * The name of the operation or command being executed.
 *
 * @example findAndModify
 * @example HMSET
 * @example SELECT
 *
 * @note It is **RECOMMENDED** to capture the value as provided by the application
 * without attempting to do any case normalization.
 *
 * The operation name **SHOULD NOT** be extracted from `db.query.text`,
 * when the database system supports query text with multiple operations
 * in non-batch operations.
 *
 * If spaces can occur in the operation name, multiple consecutive spaces
 * **SHOULD** be normalized to a single space.
 *
 * For batch operations, if the individual operations are known to have the same operation name
 * then that operation name **SHOULD** be used prepended by `BATCH `,
 * otherwise `db.operation.name` **SHOULD** be `BATCH` or some other database
 * system specific term if more applicable.
 */
const ATTR_DB_OPERATION_NAME = 'db.operation.name';
/**
 * Low cardinality summary of a database query.
 *
 * @example SELECT wuser_table
 * @example INSERT shipping_details SELECT orders
 * @example get user by id
 *
 * @note The query summary describes a class of database queries and is useful
 * as a grouping key, especially when analyzing telemetry for database
 * calls involving complex queries.
 *
 * Summary may be available to the instrumentation through
 * instrumentation hooks or other means. If it is not available, instrumentations
 * that support query parsing **SHOULD** generate a summary following
 * [Generating query summary](/docs/db/database-spans.md#generating-a-summary-of-the-query)
 * section.
 *
 * For batch operations, if the individual operations are known to have the same query summary
 * then that query summary **SHOULD** be used prepended by `BATCH `,
 * otherwise `db.query.summary` **SHOULD** be `BATCH` or some other database
 * system specific term if more applicable.
 */
const ATTR_DB_QUERY_SUMMARY = 'db.query.summary';
/**
 * The database query being executed.
 *
 * @example SELECT * FROM wuser_table where username = ?
 * @example SET mykey ?
 *
 * @note For sanitization see [Sanitization of `db.query.text`](/docs/db/database-spans.md#sanitization-of-dbquerytext).
 * For batch operations, if the individual operations are known to have the same query text then that query text **SHOULD** be used, otherwise all of the individual query texts **SHOULD** be concatenated with separator `; ` or some other database system specific separator if more applicable.
 * Parameterized query text **SHOULD NOT** be sanitized. Even though parameterized query text can potentially have sensitive data, by using a parameterized query the user is giving a strong signal that any sensitive data will be passed as parameter values, and the benefit to observability of capturing the static part of the query text by default outweighs the risk.
 */
const ATTR_DB_QUERY_TEXT = 'db.query.text';
/**
 * Database response status code.
 *
 * @example 102
 * @example ORA-17002
 * @example 08P01
 * @example 404
 *
 * @note The status code returned by the database. Usually it represents an error code, but may also represent partial success, warning, or differentiate between various types of successful outcomes.
 * Semantic conventions for individual database systems **SHOULD** document what `db.response.status_code` means in the context of that system.
 */
const ATTR_DB_RESPONSE_STATUS_CODE = 'db.response.status_code';
/**
 * The name of a stored procedure within the database.
 *
 * @example GetCustomer
 *
 * @note It is **RECOMMENDED** to capture the value as provided by the application
 * without attempting to do any case normalization.
 *
 * For batch operations, if the individual operations are known to have the same
 * stored procedure name then that stored procedure name **SHOULD** be used.
 */
const ATTR_DB_STORED_PROCEDURE_NAME = 'db.stored_procedure.name';
/**
 * The database management system (DBMS) product as identified by the client instrumentation.
 *
 * @note The actual DBMS may differ from the one identified by the client. For example, when using PostgreSQL client libraries to connect to a CockroachDB, the `db.system.name` is set to `postgresql` based on the instrumentation's best knowledge.
 */
const ATTR_DB_SYSTEM_NAME = 'db.system.name';
/**
 * Enum value "mariadb" for attribute {@link ATTR_DB_SYSTEM_NAME}.
 *
 * [MariaDB](https://mariadb.org/)
 */
const DB_SYSTEM_NAME_VALUE_MARIADB = "mariadb";
/**
 * Enum value "microsoft.sql_server" for attribute {@link ATTR_DB_SYSTEM_NAME}.
 *
 * [Microsoft SQL Server](https://www.microsoft.com/sql-server)
 */
const DB_SYSTEM_NAME_VALUE_MICROSOFT_SQL_SERVER = "microsoft.sql_server";
/**
 * Enum value "mysql" for attribute {@link ATTR_DB_SYSTEM_NAME}.
 *
 * [MySQL](https://www.mysql.com/)
 */
const DB_SYSTEM_NAME_VALUE_MYSQL = "mysql";
/**
 * Enum value "postgresql" for attribute {@link ATTR_DB_SYSTEM_NAME}.
 *
 * [PostgreSQL](https://www.postgresql.org/)
 */
const DB_SYSTEM_NAME_VALUE_POSTGRESQL = "postgresql";
/**
 * Name of the [deployment environment](https://wikipedia.org/wiki/Deployment_environment) (aka deployment tier).
 *
 * @example staging
 * @example production
 *
 * @note `deployment.environment.name` does not affect the uniqueness constraints defined through
 * the `service.namespace`, `service.name` and `service.instance.id` resource attributes.
 * This implies that resources carrying the following attribute combinations **MUST** be
 * considered to be identifying the same service:
 *
 *   - `service.name=frontend`, `deployment.environment.name=production`
 *   - `service.name=frontend`, `deployment.environment.name=staging`.
 */
const ATTR_DEPLOYMENT_ENVIRONMENT_NAME = 'deployment.environment.name';
/**
 * Enum value "development" for attribute {@link ATTR_DEPLOYMENT_ENVIRONMENT_NAME}.
 *
 * Development environment
 */
const DEPLOYMENT_ENVIRONMENT_NAME_VALUE_DEVELOPMENT = "development";
/**
 * Enum value "production" for attribute {@link ATTR_DEPLOYMENT_ENVIRONMENT_NAME}.
 *
 * Production environment
 */
const DEPLOYMENT_ENVIRONMENT_NAME_VALUE_PRODUCTION = "production";
/**
 * Enum value "staging" for attribute {@link ATTR_DEPLOYMENT_ENVIRONMENT_NAME}.
 *
 * Staging environment
 */
const DEPLOYMENT_ENVIRONMENT_NAME_VALUE_STAGING = "staging";
/**
 * Enum value "test" for attribute {@link ATTR_DEPLOYMENT_ENVIRONMENT_NAME}.
 *
 * Testing environment
 */
const DEPLOYMENT_ENVIRONMENT_NAME_VALUE_TEST = "test";
/**
 * Name of the garbage collector managed heap generation.
 *
 * @example gen0
 * @example gen1
 * @example gen2
 */
const ATTR_DOTNET_GC_HEAP_GENERATION = 'dotnet.gc.heap.generation';
/**
 * Enum value "gen0" for attribute {@link ATTR_DOTNET_GC_HEAP_GENERATION}.
 *
 * Generation 0
 */
const DOTNET_GC_HEAP_GENERATION_VALUE_GEN0 = "gen0";
/**
 * Enum value "gen1" for attribute {@link ATTR_DOTNET_GC_HEAP_GENERATION}.
 *
 * Generation 1
 */
const DOTNET_GC_HEAP_GENERATION_VALUE_GEN1 = "gen1";
/**
 * Enum value "gen2" for attribute {@link ATTR_DOTNET_GC_HEAP_GENERATION}.
 *
 * Generation 2
 */
const DOTNET_GC_HEAP_GENERATION_VALUE_GEN2 = "gen2";
/**
 * Enum value "loh" for attribute {@link ATTR_DOTNET_GC_HEAP_GENERATION}.
 *
 * Large Object Heap
 */
const DOTNET_GC_HEAP_GENERATION_VALUE_LOH = "loh";
/**
 * Enum value "poh" for attribute {@link ATTR_DOTNET_GC_HEAP_GENERATION}.
 *
 * Pinned Object Heap
 */
const DOTNET_GC_HEAP_GENERATION_VALUE_POH = "poh";
/**
 * Describes a class of error the operation ended with.
 *
 * @example timeout
 * @example java.net.UnknownHostException
 * @example server_certificate_invalid
 * @example 500
 *
 * @note The `error.type` **SHOULD** be predictable, and **SHOULD** have low cardinality.
 *
 * When `error.type` is set to a type (e.g., an exception type), its
 * canonical class name identifying the type within the artifact **SHOULD** be used.
 *
 * If the recorded error type is a wrapper that is not meaningful for
 * failure classification, instrumentation **MAY** use the type of the inner
 * error instead. For example, in Go, errors created with `fmt.Errorf`
 * using `%w` **MAY** be unwrapped when the wrapper type does not help
 * classify the failure.
 *
 * Instrumentations **SHOULD** document the list of errors they report.
 *
 * The cardinality of `error.type` within one instrumentation library **SHOULD** be low.
 * Telemetry consumers that aggregate data from multiple instrumentation libraries and applications
 * should be prepared for `error.type` to have high cardinality at query time when no
 * additional filters are applied.
 *
 * If the operation has completed successfully, instrumentations **SHOULD NOT** set `error.type`.
 *
 * If a specific domain defines its own set of error identifiers (such as HTTP or RPC status codes),
 * it's **RECOMMENDED** to:
 *
 *   - Use a domain-specific attribute
 *   - Set `error.type` to capture all errors, regardless of whether they are defined within the domain-specific set or not.
 */
const ATTR_ERROR_TYPE = 'error.type';
/**
 * Enum value "_OTHER" for attribute {@link ATTR_ERROR_TYPE}.
 *
 * A fallback error value to be used when the instrumentation doesn't define a custom value.
 */
const ERROR_TYPE_VALUE_OTHER = "_OTHER";
/**
 * Indicates that the exception is escaping the scope of the span.
 *
 * @deprecated It's no longer recommended to record exceptions that are handled and do not escape the scope of a span.
 */
const ATTR_EXCEPTION_ESCAPED = 'exception.escaped';
/**
 * The exception message.
 *
 * @example Division by zero
 * @example Can't convert 'int' object to str implicitly
 *
 * @note > [!WARNING]
 *
 * > This attribute may contain sensitive information.
 */
const ATTR_EXCEPTION_MESSAGE = 'exception.message';
/**
 * A stacktrace as a string in the natural representation for the language runtime. The representation is to be determined and documented by each language SIG.
 *
 * @example "Exception in thread "main" java.lang.RuntimeException: Test exception\\n at com.example.GenerateTrace.methodB(GenerateTrace.java:13)\\n at com.example.GenerateTrace.methodA(GenerateTrace.java:9)\\n at com.example.GenerateTrace.main(GenerateTrace.java:5)\\n"
 */
const ATTR_EXCEPTION_STACKTRACE = 'exception.stacktrace';
/**
 * The type of the exception (its fully-qualified class name, if applicable). The dynamic type of the exception should be preferred over the static type in languages that support it.
 *
 * @example java.net.ConnectException
 * @example OSError
 *
 * @note If the recorded exception type is a wrapper that is not meaningful for
 * failure classification, instrumentation **MAY** use the type of the inner
 * exception instead. For example, in Go, errors created with `fmt.Errorf`
 * using `%w` **MAY** be unwrapped when the wrapper type does not help
 * classify the failure.
 */
const ATTR_EXCEPTION_TYPE = 'exception.type';
/**
 * HTTP request headers, `<key>` being the normalized HTTP Header name (lowercase), the value being the header values.
 *
 * @example ["application/json"]
 * @example ["1.2.3.4", "1.2.3.5"]
 *
 * @note Instrumentations **SHOULD** require an explicit configuration of which headers are to be captured.
 * Including all request headers can be a security risk - explicit configuration helps avoid leaking sensitive information.
 *
 * The `User-Agent` header is already captured in the `user_agent.original` attribute.
 * Users **MAY** explicitly configure instrumentations to capture them even though it is not recommended.
 *
 * The attribute value **MUST** consist of either multiple header values as an array of strings
 * or a single-item array containing a possibly comma-concatenated string, depending on the way
 * the HTTP library provides access to headers.
 *
 * Examples:
 *
 *   - A header `Content-Type: application/json` **SHOULD** be recorded as the `http.request.header.content-type`
 *     attribute with value `["application/json"]`.
 *   - A header `X-Forwarded-For: 1.2.3.4, 1.2.3.5` **SHOULD** be recorded as the `http.request.header.x-forwarded-for`
 *     attribute with value `["1.2.3.4", "1.2.3.5"]` or `["1.2.3.4, 1.2.3.5"]` depending on the HTTP library.
 */
const ATTR_HTTP_REQUEST_HEADER = (key) => `http.request.header.${key}`;
/**
 * HTTP request method.
 *
 * @example GET
 * @example POST
 * @example HEAD
 *
 * @note HTTP request method value **SHOULD** be "known" to the instrumentation.
 * By default, this convention defines "known" methods as the ones listed in [RFC9110](https://www.rfc-editor.org/rfc/rfc9110.html#name-methods),
 * the PATCH method defined in [RFC5789](https://www.rfc-editor.org/rfc/rfc5789.html)
 * and the QUERY method defined in [httpbis-safe-method-w-body](https://datatracker.ietf.org/doc/draft-ietf-httpbis-safe-method-w-body/?include_text=1).
 *
 * If the HTTP request method is not known to instrumentation, it **MUST** set the `http.request.method` attribute to `_OTHER`.
 *
 * If the HTTP instrumentation could end up converting valid HTTP request methods to `_OTHER`, then it **MUST** provide a way to override
 * the list of known HTTP methods. If this override is done via environment variable, then the environment variable **MUST** be named
 * OTEL_INSTRUMENTATION_HTTP_KNOWN_METHODS and support a comma-separated list of case-sensitive known HTTP methods.
 *
 *
 * If this override is done via declarative configuration, then the list **MUST** be configurable via the `known_methods` property
 * (an array of case-sensitive strings with minimum items 0) under `.instrumentation/development.general.http.client` and/or
 * `.instrumentation/development.general.http.server`.
 *
 * In either case, this list **MUST** be a full override of the default known methods,
 * it is not a list of known methods in addition to the defaults.
 *
 * HTTP method names are case-sensitive and `http.request.method` attribute value **MUST** match a known HTTP method name exactly.
 * Instrumentations for specific web frameworks that consider HTTP methods to be case insensitive, **SHOULD** populate a canonical equivalent.
 * Tracing instrumentations that do so, **MUST** also set `http.request.method_original` to the original value.
 */
const ATTR_HTTP_REQUEST_METHOD = 'http.request.method';
/**
 * Enum value "_OTHER" for attribute {@link ATTR_HTTP_REQUEST_METHOD}.
 *
 * Any HTTP method that the instrumentation has no prior knowledge of.
 */
const HTTP_REQUEST_METHOD_VALUE_OTHER = "_OTHER";
/**
 * Enum value "CONNECT" for attribute {@link ATTR_HTTP_REQUEST_METHOD}.
 *
 * CONNECT method.
 */
const HTTP_REQUEST_METHOD_VALUE_CONNECT = "CONNECT";
/**
 * Enum value "DELETE" for attribute {@link ATTR_HTTP_REQUEST_METHOD}.
 *
 * DELETE method.
 */
const HTTP_REQUEST_METHOD_VALUE_DELETE = "DELETE";
/**
 * Enum value "GET" for attribute {@link ATTR_HTTP_REQUEST_METHOD}.
 *
 * GET method.
 */
const HTTP_REQUEST_METHOD_VALUE_GET = "GET";
/**
 * Enum value "HEAD" for attribute {@link ATTR_HTTP_REQUEST_METHOD}.
 *
 * HEAD method.
 */
const HTTP_REQUEST_METHOD_VALUE_HEAD = "HEAD";
/**
 * Enum value "OPTIONS" for attribute {@link ATTR_HTTP_REQUEST_METHOD}.
 *
 * OPTIONS method.
 */
const HTTP_REQUEST_METHOD_VALUE_OPTIONS = "OPTIONS";
/**
 * Enum value "PATCH" for attribute {@link ATTR_HTTP_REQUEST_METHOD}.
 *
 * PATCH method.
 */
const HTTP_REQUEST_METHOD_VALUE_PATCH = "PATCH";
/**
 * Enum value "POST" for attribute {@link ATTR_HTTP_REQUEST_METHOD}.
 *
 * POST method.
 */
const HTTP_REQUEST_METHOD_VALUE_POST = "POST";
/**
 * Enum value "PUT" for attribute {@link ATTR_HTTP_REQUEST_METHOD}.
 *
 * PUT method.
 */
const HTTP_REQUEST_METHOD_VALUE_PUT = "PUT";
/**
 * Enum value "TRACE" for attribute {@link ATTR_HTTP_REQUEST_METHOD}.
 *
 * TRACE method.
 */
const HTTP_REQUEST_METHOD_VALUE_TRACE = "TRACE";
/**
 * Original HTTP method sent by the client in the request line.
 *
 * @example GeT
 * @example ACL
 * @example foo
 */
const ATTR_HTTP_REQUEST_METHOD_ORIGINAL = 'http.request.method_original';
/**
 * The ordinal number of request resending attempt (for any reason, including redirects).
 *
 * @example 3
 *
 * @note The resend count **SHOULD** be updated each time an HTTP request gets resent by the client, regardless of what was the cause of the resending (e.g. redirection, authorization failure, 503 Server Unavailable, network issues, or any other).
 */
const ATTR_HTTP_REQUEST_RESEND_COUNT = 'http.request.resend_count';
/**
 * HTTP response headers, `<key>` being the normalized HTTP Header name (lowercase), the value being the header values.
 *
 * @example ["application/json"]
 * @example ["abc", "def"]
 *
 * @note Instrumentations **SHOULD** require an explicit configuration of which headers are to be captured.
 * Including all response headers can be a security risk - explicit configuration helps avoid leaking sensitive information.
 *
 * Users **MAY** explicitly configure instrumentations to capture them even though it is not recommended.
 *
 * The attribute value **MUST** consist of either multiple header values as an array of strings
 * or a single-item array containing a possibly comma-concatenated string, depending on the way
 * the HTTP library provides access to headers.
 *
 * Examples:
 *
 *   - A header `Content-Type: application/json` header **SHOULD** be recorded as the `http.request.response.content-type`
 *     attribute with value `["application/json"]`.
 *   - A header `My-custom-header: abc, def` header **SHOULD** be recorded as the `http.response.header.my-custom-header`
 *     attribute with value `["abc", "def"]` or `["abc, def"]` depending on the HTTP library.
 */
const ATTR_HTTP_RESPONSE_HEADER = (key) => `http.response.header.${key}`;
/**
 * [HTTP response status code](https://tools.ietf.org/html/rfc7231#section-6).
 *
 * @example 200
 */
const ATTR_HTTP_RESPONSE_STATUS_CODE = 'http.response.status_code';
/**
 * The matched route template for the request. This **MUST** be low-cardinality and include all static path segments, with dynamic path segments represented with placeholders.
 *
 * @example /users/:userID?
 * @example my-controller/my-action/{id?}
 *
 * @note **MUST NOT** be populated when this is not supported by the HTTP server framework as the route attribute should have low-cardinality and the URI path can NOT substitute it.
 * **SHOULD** include the [application root](/docs/http/http-spans.md#http-server-definitions) if there is one.
 *
 * A static path segment is a part of the route template with a fixed, low-cardinality value. This includes literal strings like `/users/` and placeholders that
 * are constrained to a finite, predefined set of values, e.g. `{controller}` or `{action}`.
 *
 * A dynamic path segment is a placeholder for a value that can have high cardinality and is not constrained to a predefined list like static path segments.
 *
 * Instrumentations **SHOULD** use routing information provided by the corresponding web framework. They **SHOULD** pick the most precise source of routing information and **MAY**
 * support custom route formatting. Instrumentations **SHOULD** document the format and the API used to obtain the route string.
 */
const ATTR_HTTP_ROUTE = 'http.route';
/**
 * Name of the garbage collector action.
 *
 * @example end of minor GC
 * @example end of major GC
 *
 * @note Garbage collector action is generally obtained via [GarbageCollectionNotificationInfo#getGcAction()](https://docs.oracle.com/en/java/javase/11/docs/api/jdk.management/com/sun/management/GarbageCollectionNotificationInfo.html#getGcAction()).
 */
const ATTR_JVM_GC_ACTION = 'jvm.gc.action';
/**
 * Name of the garbage collector.
 *
 * @example G1 Young Generation
 * @example G1 Old Generation
 *
 * @note Garbage collector name is generally obtained via [GarbageCollectionNotificationInfo#getGcName()](https://docs.oracle.com/en/java/javase/11/docs/api/jdk.management/com/sun/management/GarbageCollectionNotificationInfo.html#getGcName()).
 */
const ATTR_JVM_GC_NAME = 'jvm.gc.name';
/**
 * Name of the memory pool.
 *
 * @example G1 Old Gen
 * @example G1 Eden space
 * @example G1 Survivor Space
 *
 * @note Pool names are generally obtained via [MemoryPoolMXBean#getName()](https://docs.oracle.com/en/java/javase/11/docs/api/java.management/java/lang/management/MemoryPoolMXBean.html#getName()).
 */
const ATTR_JVM_MEMORY_POOL_NAME = 'jvm.memory.pool.name';
/**
 * The type of memory.
 *
 * @example heap
 * @example non_heap
 */
const ATTR_JVM_MEMORY_TYPE = 'jvm.memory.type';
/**
 * Enum value "heap" for attribute {@link ATTR_JVM_MEMORY_TYPE}.
 *
 * Heap memory.
 */
const JVM_MEMORY_TYPE_VALUE_HEAP = "heap";
/**
 * Enum value "non_heap" for attribute {@link ATTR_JVM_MEMORY_TYPE}.
 *
 * Non-heap memory
 */
const JVM_MEMORY_TYPE_VALUE_NON_HEAP = "non_heap";
/**
 * Whether the thread is daemon or not.
 */
const ATTR_JVM_THREAD_DAEMON = 'jvm.thread.daemon';
/**
 * State of the thread.
 *
 * @example runnable
 * @example blocked
 */
const ATTR_JVM_THREAD_STATE = 'jvm.thread.state';
/**
 * Enum value "blocked" for attribute {@link ATTR_JVM_THREAD_STATE}.
 *
 * A thread that is blocked waiting for a monitor lock is in this state.
 */
const JVM_THREAD_STATE_VALUE_BLOCKED = "blocked";
/**
 * Enum value "new" for attribute {@link ATTR_JVM_THREAD_STATE}.
 *
 * A thread that has not yet started is in this state.
 */
const JVM_THREAD_STATE_VALUE_NEW = "new";
/**
 * Enum value "runnable" for attribute {@link ATTR_JVM_THREAD_STATE}.
 *
 * A thread executing in the Java virtual machine is in this state.
 */
const JVM_THREAD_STATE_VALUE_RUNNABLE = "runnable";
/**
 * Enum value "terminated" for attribute {@link ATTR_JVM_THREAD_STATE}.
 *
 * A thread that has exited is in this state.
 */
const JVM_THREAD_STATE_VALUE_TERMINATED = "terminated";
/**
 * Enum value "timed_waiting" for attribute {@link ATTR_JVM_THREAD_STATE}.
 *
 * A thread that is waiting for another thread to perform an action for up to a specified waiting time is in this state.
 */
const JVM_THREAD_STATE_VALUE_TIMED_WAITING = "timed_waiting";
/**
 * Enum value "waiting" for attribute {@link ATTR_JVM_THREAD_STATE}.
 *
 * A thread that is waiting indefinitely for another thread to perform a particular action is in this state.
 */
const JVM_THREAD_STATE_VALUE_WAITING = "waiting";
/**
 * The name of the cluster.
 *
 * @example opentelemetry-cluster
 */
const ATTR_K8S_CLUSTER_NAME = 'k8s.cluster.name';
/**
 * A pseudo-ID for the cluster, set to the UID of the `kube-system` namespace.
 *
 * @example 218fc5a9-a5f1-4b54-aa05-46717d0ab26d
 *
 * @note K8s doesn't have support for obtaining a cluster ID. If this is ever
 * added, we will recommend collecting the `k8s.cluster.uid` through the
 * official APIs. In the meantime, we are able to use the `uid` of the
 * `kube-system` namespace as a proxy for cluster ID. Read on for the
 * rationale.
 *
 * Every object created in a K8s cluster is assigned a distinct UID. The
 * `kube-system` namespace is used by Kubernetes itself and will exist
 * for the lifetime of the cluster. Using the `uid` of the `kube-system`
 * namespace is a reasonable proxy for the K8s ClusterID as it will only
 * change if the cluster is rebuilt. Furthermore, Kubernetes UIDs are
 * UUIDs as standardized by
 * [ISO/IEC 9834-8 and ITU-T X.667](https://www.itu.int/ITU-T/studygroups/com17/oid.html).
 * Which states:
 *
 * > If generated according to one of the mechanisms defined in Rec.
 * > ITU-T X.667 | ISO/IEC 9834-8, a UUID is either guaranteed to be
 * > different from all other UUIDs generated before 3603 A.D., or is
 * > extremely likely to be different (depending on the mechanism chosen).
 *
 * Therefore, UIDs between clusters should be extremely unlikely to
 * conflict.
 */
const ATTR_K8S_CLUSTER_UID = 'k8s.cluster.uid';
/**
 * The name of the Container from Pod specification, must be unique within a Pod. Container runtime usually uses different globally unique name (`container.name`).
 *
 * @example redis
 */
const ATTR_K8S_CONTAINER_NAME = 'k8s.container.name';
/**
 * Number of times the container was restarted. This attribute can be used to identify a particular container (running or stopped) within a container spec.
 */
const ATTR_K8S_CONTAINER_RESTART_COUNT = 'k8s.container.restart_count';
/**
 * The cronjob annotation placed on the CronJob, the `<key>` being the annotation name, the value being the annotation value.
 *
 * @example 4
 * @example
 *
 * @note Examples:
 *
 *   - An annotation `retries` with value `4` **SHOULD** be recorded as the
 *     `k8s.cronjob.annotation.retries` attribute with value `"4"`.
 *   - An annotation `data` with empty string value **SHOULD** be recorded as
 *     the `k8s.cronjob.annotation.data` attribute with value `""`.
 */
const ATTR_K8S_CRONJOB_ANNOTATION = (key) => `k8s.cronjob.annotation.${key}`;
/**
 * The label placed on the CronJob, the `<key>` being the label name, the value being the label value.
 *
 * @example weekly
 * @example
 *
 * @note Examples:
 *
 *   - A label `type` with value `weekly` **SHOULD** be recorded as the
 *     `k8s.cronjob.label.type` attribute with value `"weekly"`.
 *   - A label `automated` with empty string value **SHOULD** be recorded as
 *     the `k8s.cronjob.label.automated` attribute with value `""`.
 */
const ATTR_K8S_CRONJOB_LABEL = (key) => `k8s.cronjob.label.${key}`;
/**
 * The name of the CronJob.
 *
 * @example opentelemetry
 */
const ATTR_K8S_CRONJOB_NAME = 'k8s.cronjob.name';
/**
 * The UID of the CronJob.
 *
 * @example 275ecb36-5aa8-4c2a-9c47-d8bb681b9aff
 */
const ATTR_K8S_CRONJOB_UID = 'k8s.cronjob.uid';
/**
 * The annotation placed on the DaemonSet, the `<key>` being the annotation name, the value being the annotation value, even if the value is empty.
 *
 * @example 1
 * @example
 *
 * @note
 * Examples:
 *
 *   - An annotation `replicas` with value `1` **SHOULD** be recorded
 *     as the `k8s.daemonset.annotation.replicas` attribute with value `"1"`.
 *   - An annotation `data` with empty string value **SHOULD** be recorded as
 *     the `k8s.daemonset.annotation.data` attribute with value `""`.
 */
const ATTR_K8S_DAEMONSET_ANNOTATION = (key) => `k8s.daemonset.annotation.${key}`;
/**
 * The label placed on the DaemonSet, the `<key>` being the label name, the value being the label value, even if the value is empty.
 *
 * @example guestbook
 * @example
 *
 * @note
 * Examples:
 *
 *   - A label `app` with value `guestbook` **SHOULD** be recorded
 *     as the `k8s.daemonset.label.app` attribute with value `"guestbook"`.
 *   - A label `injected` with empty string value **SHOULD** be recorded as
 *     the `k8s.daemonset.label.injected` attribute with value `""`.
 */
const ATTR_K8S_DAEMONSET_LABEL = (key) => `k8s.daemonset.label.${key}`;
/**
 * The name of the DaemonSet.
 *
 * @example opentelemetry
 */
const ATTR_K8S_DAEMONSET_NAME = 'k8s.daemonset.name';
/**
 * The UID of the DaemonSet.
 *
 * @example 275ecb36-5aa8-4c2a-9c47-d8bb681b9aff
 */
const ATTR_K8S_DAEMONSET_UID = 'k8s.daemonset.uid';
/**
 * The annotation placed on the Deployment, the `<key>` being the annotation name, the value being the annotation value, even if the value is empty.
 *
 * @example 1
 * @example
 *
 * @note
 * Examples:
 *
 *   - An annotation `replicas` with value `1` **SHOULD** be recorded
 *     as the `k8s.deployment.annotation.replicas` attribute with value `"1"`.
 *   - An annotation `data` with empty string value **SHOULD** be recorded as
 *     the `k8s.deployment.annotation.data` attribute with value `""`.
 */
const ATTR_K8S_DEPLOYMENT_ANNOTATION = (key) => `k8s.deployment.annotation.${key}`;
/**
 * The label placed on the Deployment, the `<key>` being the label name, the value being the label value, even if the value is empty.
 *
 * @example guestbook
 * @example
 *
 * @note
 * Examples:
 *
 *   - A label `app` with value `guestbook` **SHOULD** be recorded
 *     as the `k8s.deployment.label.app` attribute with value `"guestbook"`.
 *   - A label `injected` with empty string value **SHOULD** be recorded as
 *     the `k8s.deployment.label.injected` attribute with value `""`.
 */
const ATTR_K8S_DEPLOYMENT_LABEL = (key) => `k8s.deployment.label.${key}`;
/**
 * The name of the Deployment.
 *
 * @example opentelemetry
 */
const ATTR_K8S_DEPLOYMENT_NAME = 'k8s.deployment.name';
/**
 * The UID of the Deployment.
 *
 * @example 275ecb36-5aa8-4c2a-9c47-d8bb681b9aff
 */
const ATTR_K8S_DEPLOYMENT_UID = 'k8s.deployment.uid';
/**
 * The annotation placed on the Job, the `<key>` being the annotation name, the value being the annotation value, even if the value is empty.
 *
 * @example 1
 * @example
 *
 * @note
 * Examples:
 *
 *   - An annotation `number` with value `1` **SHOULD** be recorded
 *     as the `k8s.job.annotation.number` attribute with value `"1"`.
 *   - An annotation `data` with empty string value **SHOULD** be recorded as
 *     the `k8s.job.annotation.data` attribute with value `""`.
 */
const ATTR_K8S_JOB_ANNOTATION = (key) => `k8s.job.annotation.${key}`;
/**
 * The label placed on the Job, the `<key>` being the label name, the value being the label value, even if the value is empty.
 *
 * @example ci
 * @example
 *
 * @note
 * Examples:
 *
 *   - A label `jobtype` with value `ci` **SHOULD** be recorded
 *     as the `k8s.job.label.jobtype` attribute with value `"ci"`.
 *   - A label `automated` with empty string value **SHOULD** be recorded as
 *     the `k8s.job.label.automated` attribute with value `""`.
 */
const ATTR_K8S_JOB_LABEL = (key) => `k8s.job.label.${key}`;
/**
 * The name of the Job.
 *
 * @example opentelemetry
 */
const ATTR_K8S_JOB_NAME = 'k8s.job.name';
/**
 * The UID of the Job.
 *
 * @example 275ecb36-5aa8-4c2a-9c47-d8bb681b9aff
 */
const ATTR_K8S_JOB_UID = 'k8s.job.uid';
/**
 * The annotation placed on the Namespace, the `<key>` being the annotation name, the value being the annotation value, even if the value is empty.
 *
 * @example 0
 * @example
 *
 * @note
 * Examples:
 *
 *   - An annotation `ttl` with value `0` **SHOULD** be recorded
 *     as the `k8s.namespace.annotation.ttl` attribute with value `"0"`.
 *   - An annotation `data` with empty string value **SHOULD** be recorded as
 *     the `k8s.namespace.annotation.data` attribute with value `""`.
 */
const ATTR_K8S_NAMESPACE_ANNOTATION = (key) => `k8s.namespace.annotation.${key}`;
/**
 * The label placed on the Namespace, the `<key>` being the label name, the value being the label value, even if the value is empty.
 *
 * @example default
 * @example
 *
 * @note
 * Examples:
 *
 *   - A label `kubernetes.io/metadata.name` with value `default` **SHOULD** be recorded
 *     as the `k8s.namespace.label.kubernetes.io/metadata.name` attribute with value `"default"`.
 *   - A label `data` with empty string value **SHOULD** be recorded as
 *     the `k8s.namespace.label.data` attribute with value `""`.
 */
const ATTR_K8S_NAMESPACE_LABEL = (key) => `k8s.namespace.label.${key}`;
/**
 * The name of the namespace that the pod is running in.
 *
 * @example default
 */
const ATTR_K8S_NAMESPACE_NAME = 'k8s.namespace.name';
/**
 * The annotation placed on the Node, the `<key>` being the annotation name, the value being the annotation value, even if the value is empty.
 *
 * @example 0
 * @example
 *
 * @note Examples:
 *
 *   - An annotation `node.alpha.kubernetes.io/ttl` with value `0` **SHOULD** be recorded as
 *     the `k8s.node.annotation.node.alpha.kubernetes.io/ttl` attribute with value `"0"`.
 *   - An annotation `data` with empty string value **SHOULD** be recorded as
 *     the `k8s.node.annotation.data` attribute with value `""`.
 */
const ATTR_K8S_NODE_ANNOTATION = (key) => `k8s.node.annotation.${key}`;
/**
 * The label placed on the Node, the `<key>` being the label name, the value being the label value, even if the value is empty.
 *
 * @example arm64
 * @example
 *
 * @note Examples:
 *
 *   - A label `kubernetes.io/arch` with value `arm64` **SHOULD** be recorded
 *     as the `k8s.node.label.kubernetes.io/arch` attribute with value `"arm64"`.
 *   - A label `data` with empty string value **SHOULD** be recorded as
 *     the `k8s.node.label.data` attribute with value `""`.
 */
const ATTR_K8S_NODE_LABEL = (key) => `k8s.node.label.${key}`;
/**
 * The name of the Node.
 *
 * @example node-1
 */
const ATTR_K8S_NODE_NAME = 'k8s.node.name';
/**
 * The UID of the Node.
 *
 * @example 1eb3a0c6-0477-4080-a9cb-0cb7db65c6a2
 */
const ATTR_K8S_NODE_UID = 'k8s.node.uid';
/**
 * The annotation placed on the Pod, the `<key>` being the annotation name, the value being the annotation value.
 *
 * @example true
 * @example x64
 * @example
 *
 * @note Examples:
 *
 *   - An annotation `kubernetes.io/enforce-mountable-secrets` with value `true` **SHOULD** be recorded as
 *     the `k8s.pod.annotation.kubernetes.io/enforce-mountable-secrets` attribute with value `"true"`.
 *   - An annotation `mycompany.io/arch` with value `x64` **SHOULD** be recorded as
 *     the `k8s.pod.annotation.mycompany.io/arch` attribute with value `"x64"`.
 *   - An annotation `data` with empty string value **SHOULD** be recorded as
 *     the `k8s.pod.annotation.data` attribute with value `""`.
 */
const ATTR_K8S_POD_ANNOTATION = (key) => `k8s.pod.annotation.${key}`;
/**
 * Specifies the hostname of the Pod.
 *
 * @example collector-gateway
 *
 * @note The K8s Pod spec has an optional hostname field, which can be used to specify a hostname.
 * Refer to [K8s docs](https://kubernetes.io/docs/concepts/services-networking/dns-pod-service/#pod-hostname-and-subdomain-field)
 * for more information about this field.
 *
 * This attribute aligns with the `hostname` field of the
 * [K8s PodSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#podspec-v1-core).
 */
const ATTR_K8S_POD_HOSTNAME = 'k8s.pod.hostname';
/**
 * IP address allocated to the Pod.
 *
 * @example 172.18.0.2
 *
 * @note This attribute aligns with the `podIP` field of the
 * [K8s PodStatus](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#podstatus-v1-core).
 */
const ATTR_K8S_POD_IP = 'k8s.pod.ip';
/**
 * The label placed on the Pod, the `<key>` being the label name, the value being the label value.
 *
 * @example my-app
 * @example x64
 * @example
 *
 * @note Examples:
 *
 *   - A label `app` with value `my-app` **SHOULD** be recorded as
 *     the `k8s.pod.label.app` attribute with value `"my-app"`.
 *   - A label `mycompany.io/arch` with value `x64` **SHOULD** be recorded as
 *     the `k8s.pod.label.mycompany.io/arch` attribute with value `"x64"`.
 *   - A label `data` with empty string value **SHOULD** be recorded as
 *     the `k8s.pod.label.data` attribute with value `""`.
 */
const ATTR_K8S_POD_LABEL = (key) => `k8s.pod.label.${key}`;
/**
 * The name of the Pod.
 *
 * @example opentelemetry-pod-autoconf
 */
const ATTR_K8S_POD_NAME = 'k8s.pod.name';
/**
 * The start timestamp of the Pod.
 *
 * @example 2025-12-04T08:41:03Z
 *
 * @note Date and time at which the object was acknowledged by the Kubelet.
 * This is before the Kubelet pulled the container image(s) for the pod.
 *
 * This attribute aligns with the `startTime` field of the
 * [K8s PodStatus](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#podstatus-v1-core),
 * in ISO 8601 (RFC 3339 compatible) format.
 */
const ATTR_K8S_POD_START_TIME = 'k8s.pod.start_time';
/**
 * The UID of the Pod.
 *
 * @example 275ecb36-5aa8-4c2a-9c47-d8bb681b9aff
 */
const ATTR_K8S_POD_UID = 'k8s.pod.uid';
/**
 * The annotation placed on the ReplicaSet, the `<key>` being the annotation name, the value being the annotation value, even if the value is empty.
 *
 * @example 0
 * @example
 *
 * @note
 * Examples:
 *
 *   - An annotation `replicas` with value `0` **SHOULD** be recorded
 *     as the `k8s.replicaset.annotation.replicas` attribute with value `"0"`.
 *   - An annotation `data` with empty string value **SHOULD** be recorded as
 *     the `k8s.replicaset.annotation.data` attribute with value `""`.
 */
const ATTR_K8S_REPLICASET_ANNOTATION = (key) => `k8s.replicaset.annotation.${key}`;
/**
 * The label placed on the ReplicaSet, the `<key>` being the label name, the value being the label value, even if the value is empty.
 *
 * @example guestbook
 * @example
 *
 * @note
 * Examples:
 *
 *   - A label `app` with value `guestbook` **SHOULD** be recorded
 *     as the `k8s.replicaset.label.app` attribute with value `"guestbook"`.
 *   - A label `injected` with empty string value **SHOULD** be recorded as
 *     the `k8s.replicaset.label.injected` attribute with value `""`.
 */
const ATTR_K8S_REPLICASET_LABEL = (key) => `k8s.replicaset.label.${key}`;
/**
 * The name of the ReplicaSet.
 *
 * @example opentelemetry
 */
const ATTR_K8S_REPLICASET_NAME = 'k8s.replicaset.name';
/**
 * The UID of the ReplicaSet.
 *
 * @example 275ecb36-5aa8-4c2a-9c47-d8bb681b9aff
 */
const ATTR_K8S_REPLICASET_UID = 'k8s.replicaset.uid';
/**
 * The annotation placed on the StatefulSet, the `<key>` being the annotation name, the value being the annotation value, even if the value is empty.
 *
 * @example 1
 * @example
 *
 * @note
 * Examples:
 *
 *   - An annotation `replicas` with value `1` **SHOULD** be recorded
 *     as the `k8s.statefulset.annotation.replicas` attribute with value `"1"`.
 *   - An annotation `data` with empty string value **SHOULD** be recorded as
 *     the `k8s.statefulset.annotation.data` attribute with value `""`.
 */
const ATTR_K8S_STATEFULSET_ANNOTATION = (key) => `k8s.statefulset.annotation.${key}`;
/**
 * The label placed on the StatefulSet, the `<key>` being the label name, the value being the label value, even if the value is empty.
 *
 * @example guestbook
 * @example
 *
 * @note
 * Examples:
 *
 *   - A label `app` with value `guestbook` **SHOULD** be recorded
 *     as the `k8s.statefulset.label.app` attribute with value `"guestbook"`.
 *   - A label `injected` with empty string value **SHOULD** be recorded as
 *     the `k8s.statefulset.label.injected` attribute with value `""`.
 */
const ATTR_K8S_STATEFULSET_LABEL = (key) => `k8s.statefulset.label.${key}`;
/**
 * The name of the StatefulSet.
 *
 * @example opentelemetry
 */
const ATTR_K8S_STATEFULSET_NAME = 'k8s.statefulset.name';
/**
 * The UID of the StatefulSet.
 *
 * @example 275ecb36-5aa8-4c2a-9c47-d8bb681b9aff
 */
const ATTR_K8S_STATEFULSET_UID = 'k8s.statefulset.uid';
/**
 * Local address of the network connection - IP address or Unix domain socket name.
 *
 * @example 10.1.2.80
 * @example /tmp/my.sock
 */
const ATTR_NETWORK_LOCAL_ADDRESS = 'network.local.address';
/**
 * Local port number of the network connection.
 *
 * @example 65123
 */
const ATTR_NETWORK_LOCAL_PORT = 'network.local.port';
/**
 * Peer address of the network connection - IP address or Unix domain socket name.
 *
 * @example 10.1.2.80
 * @example /tmp/my.sock
 */
const ATTR_NETWORK_PEER_ADDRESS = 'network.peer.address';
/**
 * Peer port number of the network connection.
 *
 * @example 65123
 */
const ATTR_NETWORK_PEER_PORT = 'network.peer.port';
/**
 * [OSI application layer](https://wikipedia.org/wiki/Application_layer) or non-OSI equivalent.
 *
 * @example amqp
 * @example http
 * @example mqtt
 *
 * @note The value **SHOULD** be normalized to lowercase.
 */
const ATTR_NETWORK_PROTOCOL_NAME = 'network.protocol.name';
/**
 * The actual version of the protocol used for network communication.
 *
 * @example 1.1
 * @example 2
 *
 * @note If protocol version is subject to negotiation (for example using [ALPN](https://www.rfc-editor.org/rfc/rfc7301.html)), this attribute **SHOULD** be set to the negotiated version. If the actual protocol version is not known, this attribute **SHOULD NOT** be set.
 */
const ATTR_NETWORK_PROTOCOL_VERSION = 'network.protocol.version';
/**
 * [OSI transport layer](https://wikipedia.org/wiki/Transport_layer) or [inter-process communication method](https://wikipedia.org/wiki/Inter-process_communication).
 *
 * @example tcp
 * @example udp
 *
 * @note The value **SHOULD** be normalized to lowercase.
 *
 * Consider always setting the transport when setting a port number, since
 * a port number is ambiguous without knowing the transport. For example
 * different processes could be listening on TCP port 12345 and UDP port 12345.
 */
const ATTR_NETWORK_TRANSPORT = 'network.transport';
/**
 * Enum value "pipe" for attribute {@link ATTR_NETWORK_TRANSPORT}.
 *
 * Named or anonymous pipe.
 */
const NETWORK_TRANSPORT_VALUE_PIPE = "pipe";
/**
 * Enum value "quic" for attribute {@link ATTR_NETWORK_TRANSPORT}.
 *
 * QUIC
 */
const NETWORK_TRANSPORT_VALUE_QUIC = "quic";
/**
 * Enum value "tcp" for attribute {@link ATTR_NETWORK_TRANSPORT}.
 *
 * TCP
 */
const NETWORK_TRANSPORT_VALUE_TCP = "tcp";
/**
 * Enum value "udp" for attribute {@link ATTR_NETWORK_TRANSPORT}.
 *
 * UDP
 */
const NETWORK_TRANSPORT_VALUE_UDP = "udp";
/**
 * Enum value "unix" for attribute {@link ATTR_NETWORK_TRANSPORT}.
 *
 * Unix domain socket
 */
const NETWORK_TRANSPORT_VALUE_UNIX = "unix";
/**
 * [OSI network layer](https://wikipedia.org/wiki/Network_layer) or non-OSI equivalent.
 *
 * @example ipv4
 * @example ipv6
 *
 * @note The value **SHOULD** be normalized to lowercase.
 */
const ATTR_NETWORK_TYPE = 'network.type';
/**
 * Enum value "ipv4" for attribute {@link ATTR_NETWORK_TYPE}.
 *
 * IPv4
 */
const NETWORK_TYPE_VALUE_IPV4 = "ipv4";
/**
 * Enum value "ipv6" for attribute {@link ATTR_NETWORK_TYPE}.
 *
 * IPv6
 */
const NETWORK_TYPE_VALUE_IPV6 = "ipv6";
/**
 * Identifies the class / type of event.
 *
 * @example browser.mouse.click
 * @example device.app.lifecycle
 *
 * @note This attribute **SHOULD** be used by non-OTLP exporters when destination does not support `EventName` or equivalent field. This attribute **MAY** be used by applications using existing logging libraries so that it can be used to set the `EventName` field by Collector or SDK components.
 */
const ATTR_OTEL_EVENT_NAME = 'otel.event.name';
/**
 * The name of the instrumentation scope - (`InstrumentationScope.Name` in OTLP).
 *
 * @example io.opentelemetry.contrib.mongodb
 */
const ATTR_OTEL_SCOPE_NAME = 'otel.scope.name';
/**
 * The version of the instrumentation scope - (`InstrumentationScope.Version` in OTLP).
 *
 * @example 1.0.0
 */
const ATTR_OTEL_SCOPE_VERSION = 'otel.scope.version';
/**
 * Name of the code, either "OK" or "ERROR". **MUST NOT** be set if the status code is UNSET.
 */
const ATTR_OTEL_STATUS_CODE = 'otel.status_code';
/**
 * Enum value "ERROR" for attribute {@link ATTR_OTEL_STATUS_CODE}.
 *
 * The operation contains an error.
 */
const OTEL_STATUS_CODE_VALUE_ERROR = "ERROR";
/**
 * Enum value "OK" for attribute {@link ATTR_OTEL_STATUS_CODE}.
 *
 * The operation has been validated by an Application developer or Operator to have completed successfully.
 */
const OTEL_STATUS_CODE_VALUE_OK = "OK";
/**
 * Description of the Status if it has a value, otherwise not set.
 *
 * @example resource not found
 */
const ATTR_OTEL_STATUS_DESCRIPTION = 'otel.status_description';
/**
 * Server domain name if available without reverse DNS lookup; otherwise, IP address or Unix domain socket name.
 *
 * @example example.com
 * @example 10.1.2.80
 * @example /tmp/my.sock
 *
 * @note When observed from the client side, and when communicating through an intermediary, `server.address` **SHOULD** represent the server address behind any intermediaries, for example proxies, if it's available.
 */
const ATTR_SERVER_ADDRESS = 'server.address';
/**
 * Server port number.
 *
 * @example 80
 * @example 8080
 * @example 443
 *
 * @note When observed from the client side, and when communicating through an intermediary, `server.port` **SHOULD** represent the server port behind any intermediaries, for example proxies, if it's available.
 */
const ATTR_SERVER_PORT = 'server.port';
/**
 * The string ID of the service instance.
 *
 * @example 627cc493-f310-47de-96bd-71410b7dec09
 *
 * @note **MUST** be unique for each instance of the same `service.namespace,service.name` pair (in other words
 * `service.namespace,service.name,service.instance.id` triplet **MUST** be globally unique). The ID helps to
 * distinguish instances of the same service that exist at the same time (e.g. instances of a horizontally scaled
 * service).
 *
 * Implementations, such as SDKs, are recommended to generate a random Version 1 or Version 4 [RFC
 * 4122](https://www.ietf.org/rfc/rfc4122.txt) UUID, but are free to use an inherent unique ID as the source of
 * this value if stability is desirable. In that case, the ID **SHOULD** be used as source of a UUID Version 5 and
 * **SHOULD** use the following UUID as the namespace: `4d63009a-8d0f-11ee-aad7-4c796ed8e320`.
 *
 * UUIDs are typically recommended, as only an opaque value for the purposes of identifying a service instance is
 * needed. Similar to what can be seen in the man page for the
 * [`/etc/machine-id`](https://www.freedesktop.org/software/systemd/man/latest/machine-id.html) file, the underlying
 * data, such as pod name and namespace should be treated as confidential, being the user's choice to expose it
 * or not via another resource attribute.
 *
 * For applications running behind an application server (like unicorn), we do not recommend using one identifier
 * for all processes participating in the application. Instead, it's recommended each division (e.g. a worker
 * thread in unicorn) to have its own instance.id.
 *
 * It's not recommended for a Collector to set `service.instance.id` if it can't unambiguously determine the
 * service instance that is generating that telemetry. For instance, creating an UUID based on `pod.name` will
 * likely be wrong, as the Collector might not know from which container within that pod the telemetry originated.
 * However, Collectors can set the `service.instance.id` if they can unambiguously determine the service instance
 * for that telemetry. This is typically the case for scraping receivers, as they know the target address and
 * port.
 */
const ATTR_SERVICE_INSTANCE_ID = 'service.instance.id';
/**
 * Logical name of the service.
 *
 * @example shoppingcart
 *
 * @note **MUST** be the same for all instances of horizontally scaled services. If the value was not specified, SDKs **MUST** fallback to `unknown_service:` concatenated with the process executable name, e.g. `unknown_service:bash`. If the process executable name is not available, the value **MUST** be set to `unknown_service`.
 * The process executable name is the name of the process executable, the same value as described by the [`process.executable.name`](process.md) resource attribute.
 */
const ATTR_SERVICE_NAME = 'service.name';
/**
 * A namespace for `service.name`.
 *
 * @example Shop
 *
 * @note A string value having a meaning that helps to distinguish a group of services, for example the team name that owns a group of services. `service.name` is expected to be unique within the same namespace. If `service.namespace` is not specified in the Resource then `service.name` is expected to be unique for all services that have no explicit namespace defined (so the empty/unspecified namespace is simply one more valid namespace). Zero-length namespace string is assumed equal to unspecified namespace.
 */
const ATTR_SERVICE_NAMESPACE = 'service.namespace';
/**
 * The version string of the service component. The format is not defined by these conventions.
 *
 * @example 2.0.0
 * @example a01dbef8a
 */
const ATTR_SERVICE_VERSION = 'service.version';
/**
 * SignalR HTTP connection closure status.
 *
 * @example app_shutdown
 * @example timeout
 */
const ATTR_SIGNALR_CONNECTION_STATUS = 'signalr.connection.status';
/**
 * Enum value "app_shutdown" for attribute {@link ATTR_SIGNALR_CONNECTION_STATUS}.
 *
 * The connection was closed because the app is shutting down.
 */
const SIGNALR_CONNECTION_STATUS_VALUE_APP_SHUTDOWN = "app_shutdown";
/**
 * Enum value "normal_closure" for attribute {@link ATTR_SIGNALR_CONNECTION_STATUS}.
 *
 * The connection was closed normally.
 */
const SIGNALR_CONNECTION_STATUS_VALUE_NORMAL_CLOSURE = "normal_closure";
/**
 * Enum value "timeout" for attribute {@link ATTR_SIGNALR_CONNECTION_STATUS}.
 *
 * The connection was closed due to a timeout.
 */
const SIGNALR_CONNECTION_STATUS_VALUE_TIMEOUT = "timeout";
/**
 * [SignalR transport type](https://github.com/dotnet/aspnetcore/blob/main/src/SignalR/docs/specs/TransportProtocols.md)
 *
 * @example web_sockets
 * @example long_polling
 */
const ATTR_SIGNALR_TRANSPORT = 'signalr.transport';
/**
 * Enum value "long_polling" for attribute {@link ATTR_SIGNALR_TRANSPORT}.
 *
 * LongPolling protocol
 */
const SIGNALR_TRANSPORT_VALUE_LONG_POLLING = "long_polling";
/**
 * Enum value "server_sent_events" for attribute {@link ATTR_SIGNALR_TRANSPORT}.
 *
 * ServerSentEvents protocol
 */
const SIGNALR_TRANSPORT_VALUE_SERVER_SENT_EVENTS = "server_sent_events";
/**
 * Enum value "web_sockets" for attribute {@link ATTR_SIGNALR_TRANSPORT}.
 *
 * WebSockets protocol
 */
const SIGNALR_TRANSPORT_VALUE_WEB_SOCKETS = "web_sockets";
/**
 * The name of the auto instrumentation agent or distribution, if used.
 *
 * @example parts-unlimited-java
 *
 * @note Official auto instrumentation agents and distributions **SHOULD** set the `telemetry.distro.name` attribute to
 * a string starting with `opentelemetry-`, e.g. `opentelemetry-java-instrumentation`.
 */
const ATTR_TELEMETRY_DISTRO_NAME = 'telemetry.distro.name';
/**
 * The version string of the auto instrumentation agent or distribution, if used.
 *
 * @example 1.2.3
 */
const ATTR_TELEMETRY_DISTRO_VERSION = 'telemetry.distro.version';
/**
 * The language of the telemetry SDK.
 */
const ATTR_TELEMETRY_SDK_LANGUAGE = 'telemetry.sdk.language';
/**
 * Enum value "cpp" for attribute {@link ATTR_TELEMETRY_SDK_LANGUAGE}.
 */
const TELEMETRY_SDK_LANGUAGE_VALUE_CPP = "cpp";
/**
 * Enum value "dotnet" for attribute {@link ATTR_TELEMETRY_SDK_LANGUAGE}.
 */
const TELEMETRY_SDK_LANGUAGE_VALUE_DOTNET = "dotnet";
/**
 * Enum value "erlang" for attribute {@link ATTR_TELEMETRY_SDK_LANGUAGE}.
 */
const TELEMETRY_SDK_LANGUAGE_VALUE_ERLANG = "erlang";
/**
 * Enum value "go" for attribute {@link ATTR_TELEMETRY_SDK_LANGUAGE}.
 */
const TELEMETRY_SDK_LANGUAGE_VALUE_GO = "go";
/**
 * Enum value "java" for attribute {@link ATTR_TELEMETRY_SDK_LANGUAGE}.
 */
const TELEMETRY_SDK_LANGUAGE_VALUE_JAVA = "java";
/**
 * Enum value "kotlin" for attribute {@link ATTR_TELEMETRY_SDK_LANGUAGE}.
 */
const TELEMETRY_SDK_LANGUAGE_VALUE_KOTLIN = "kotlin";
/**
 * Enum value "nodejs" for attribute {@link ATTR_TELEMETRY_SDK_LANGUAGE}.
 */
const TELEMETRY_SDK_LANGUAGE_VALUE_NODEJS = "nodejs";
/**
 * Enum value "php" for attribute {@link ATTR_TELEMETRY_SDK_LANGUAGE}.
 */
const TELEMETRY_SDK_LANGUAGE_VALUE_PHP = "php";
/**
 * Enum value "python" for attribute {@link ATTR_TELEMETRY_SDK_LANGUAGE}.
 */
const TELEMETRY_SDK_LANGUAGE_VALUE_PYTHON = "python";
/**
 * Enum value "ruby" for attribute {@link ATTR_TELEMETRY_SDK_LANGUAGE}.
 */
const TELEMETRY_SDK_LANGUAGE_VALUE_RUBY = "ruby";
/**
 * Enum value "rust" for attribute {@link ATTR_TELEMETRY_SDK_LANGUAGE}.
 */
const TELEMETRY_SDK_LANGUAGE_VALUE_RUST = "rust";
/**
 * Enum value "swift" for attribute {@link ATTR_TELEMETRY_SDK_LANGUAGE}.
 */
const TELEMETRY_SDK_LANGUAGE_VALUE_SWIFT = "swift";
/**
 * Enum value "webjs" for attribute {@link ATTR_TELEMETRY_SDK_LANGUAGE}.
 */
const TELEMETRY_SDK_LANGUAGE_VALUE_WEBJS = "webjs";
/**
 * The name of the telemetry SDK as defined above.
 *
 * @example opentelemetry
 *
 * @note The OpenTelemetry SDK **MUST** set the `telemetry.sdk.name` attribute to `opentelemetry`.
 * If another SDK, like a fork or a vendor-provided implementation, is used, this SDK **MUST** set the
 * `telemetry.sdk.name` attribute to the fully-qualified class or module name of this SDK's main entry point
 * or another suitable identifier depending on the language.
 * The identifier `opentelemetry` is reserved and **MUST NOT** be used in this case.
 * All custom identifiers **SHOULD** be stable across different versions of an implementation.
 */
const ATTR_TELEMETRY_SDK_NAME = 'telemetry.sdk.name';
/**
 * The version string of the telemetry SDK.
 *
 * @example 1.2.3
 */
const ATTR_TELEMETRY_SDK_VERSION = 'telemetry.sdk.version';
/**
 * The [URI fragment](https://www.rfc-editor.org/rfc/rfc3986#section-3.5) component
 *
 * @example SemConv
 */
const ATTR_URL_FRAGMENT = 'url.fragment';
/**
 * Absolute URL describing a network resource according to [RFC3986](https://www.rfc-editor.org/rfc/rfc3986)
 *
 * @example https://www.foo.bar/search?q=OpenTelemetry#SemConv
 * @example //localhost
 *
 * @note For network calls, URL usually has `scheme://host[:port][path][?query][#fragment]` format, where the fragment
 * is not transmitted over HTTP, but if it is known, it **SHOULD** be included nevertheless.
 *
 * `url.full` **MUST NOT** contain credentials passed via URL in form of `https://username:password@www.example.com/`.
 * In such case username and password **SHOULD** be redacted and attribute's value **SHOULD** be `https://REDACTED:REDACTED@www.example.com/`.
 *
 * `url.full` **SHOULD** capture the absolute URL when it is available (or can be reconstructed).
 *
 * Sensitive content provided in `url.full` **SHOULD** be scrubbed when instrumentations can identify it.
 *
 *
 * Query string values for the following keys **SHOULD** be redacted by default and replaced by the
 * value `REDACTED`:
 *
 *   - [`X-Amz-Signature`](https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_sigv-authentication-methods.html)
 *   - [`X-Amz-Credential`](https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_sigv-authentication-methods.html)
 *   - [`X-Amz-Security-Token`](https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_sigv-authentication-methods.html)
 *   - [`sig`](https://learn.microsoft.com/azure/storage/common/storage-sas-overview#sas-token)
 *   - [`X-Goog-Signature`](https://cloud.google.com/storage/docs/access-control/signed-urls)
 *
 * This list is subject to change over time.
 *
 * Matching of query parameter keys against the sensitive list **SHOULD** be case-sensitive.
 *
 *
 * Instrumentation **MAY** provide a way to override this list via declarative configuration.
 * If so, it **SHOULD** use the `sensitive_query_parameters` property
 * (an array of case-sensitive strings with minimum items 0) under
 * `.instrumentation/development.general.sanitization.url`.
 * This list is a full override of the default sensitive query parameter keys,
 * it is not a list of keys in addition to the defaults.
 *
 * When a query string value is redacted, the query string key **SHOULD** still be preserved, e.g.
 * `https://www.example.com/path?color=blue&sig=REDACTED`.
 */
const ATTR_URL_FULL = 'url.full';
/**
 * The [URI path](https://www.rfc-editor.org/rfc/rfc3986#section-3.3) component
 *
 * @example /search
 *
 * @note Sensitive content provided in `url.path` **SHOULD** be scrubbed when instrumentations can identify it.
 */
const ATTR_URL_PATH = 'url.path';
/**
 * The [URI query](https://www.rfc-editor.org/rfc/rfc3986#section-3.4) component
 *
 * @example q=OpenTelemetry
 *
 * @note Sensitive content provided in `url.query` **SHOULD** be scrubbed when instrumentations can identify it.
 *
 *
 * Query string values for the following keys **SHOULD** be redacted by default and replaced by the value `REDACTED`:
 *
 *   - [`X-Amz-Signature`](https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_sigv-authentication-methods.html)
 *   - [`X-Amz-Credential`](https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_sigv-authentication-methods.html)
 *   - [`X-Amz-Security-Token`](https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_sigv-authentication-methods.html)
 *   - [`sig`](https://learn.microsoft.com/azure/storage/common/storage-sas-overview#sas-token)
 *   - [`X-Goog-Signature`](https://cloud.google.com/storage/docs/access-control/signed-urls)
 *
 * This list is subject to change over time.
 *
 * Matching of query parameter keys against the sensitive list **SHOULD** be case-sensitive.
 *
 * Instrumentation **MAY** provide a way to override this list via declarative configuration.
 * If so, it **SHOULD** use the `sensitive_query_parameters` property
 * (an array of case-sensitive strings with minimum items 0) under
 * `.instrumentation/development.general.sanitization.url`.
 * This list is a full override of the default sensitive query parameter keys,
 * it is not a list of keys in addition to the defaults.
 *
 * When a query string value is redacted, the query string key **SHOULD** still be preserved, e.g.
 * `q=OpenTelemetry&sig=REDACTED`.
 */
const ATTR_URL_QUERY = 'url.query';
/**
 * The [URI scheme](https://www.rfc-editor.org/rfc/rfc3986#section-3.1) component identifying the used protocol.
 *
 * @example https
 * @example ftp
 * @example telnet
 */
const ATTR_URL_SCHEME = 'url.scheme';
/**
 * Value of the [HTTP User-Agent](https://www.rfc-editor.org/rfc/rfc9110.html#field.user-agent) header sent by the client.
 *
 * @example CERN-LineMode/2.15 libwww/2.17b3
 * @example Mozilla/5.0 (iPhone; CPU iPhone OS 14_7_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/14.1.2 Mobile/15E148 Safari/604.1
 * @example YourApp/1.0.0 grpc-java-okhttp/1.27.2
 */
const ATTR_USER_AGENT_ORIGINAL = 'user_agent.original';
//# sourceMappingURL=stable_attributes.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+instrumentation-document-load@0.67.0+4294d048f15f6cd7/node_modules/@opentelemetry/instrumentation-document-load/build/esm/enums/AttributeNames.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
var AttributeNames;
(function (AttributeNames) {
    AttributeNames["DOCUMENT_LOAD"] = "documentLoad";
    AttributeNames["DOCUMENT_FETCH"] = "documentFetch";
    AttributeNames["RESOURCE_FETCH"] = "resourceFetch";
})(AttributeNames || (AttributeNames = {}));
//# sourceMappingURL=AttributeNames.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+instrumentation-document-load@0.67.0+4294d048f15f6cd7/node_modules/@opentelemetry/instrumentation-document-load/build/esm/version.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
// this is autogenerated file, see scripts/version-update.js
const PACKAGE_VERSION = '0.67.0';
const PACKAGE_NAME = '@opentelemetry/instrumentation-document-load';
//# sourceMappingURL=version.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+instrumentation-document-load@0.67.0+4294d048f15f6cd7/node_modules/@opentelemetry/instrumentation-document-load/build/esm/enums/EventNames.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
var EventNames;
(function (EventNames) {
    EventNames["FIRST_PAINT"] = "firstPaint";
    EventNames["FIRST_CONTENTFUL_PAINT"] = "firstContentfulPaint";
})(EventNames || (EventNames = {}));
//# sourceMappingURL=EventNames.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+instrumentation-document-load@0.67.0+4294d048f15f6cd7/node_modules/@opentelemetry/instrumentation-document-load/build/esm/utils.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */



const getPerformanceNavigationEntries = () => {
    const entries = {};
    const performanceNavigationTiming = otperformance.getEntriesByType?.('navigation')[0];
    if (performanceNavigationTiming) {
        const keys = Object.values(PerformanceTimingNames);
        keys.forEach((key) => {
            if (hasKey(performanceNavigationTiming, key)) {
                const value = performanceNavigationTiming[key];
                if (typeof value === 'number') {
                    entries[key] = value;
                }
            }
        });
    }
    else {
        // // fallback to previous version
        const perf = otperformance;
        const performanceTiming = perf.timing;
        if (performanceTiming) {
            const keys = Object.values(PerformanceTimingNames);
            keys.forEach((key) => {
                if (hasKey(performanceTiming, key)) {
                    const value = performanceTiming[key];
                    if (typeof value === 'number') {
                        entries[key] = value;
                    }
                }
            });
        }
    }
    return entries;
};
const performancePaintNames = {
    'first-paint': EventNames.FIRST_PAINT,
    'first-contentful-paint': EventNames.FIRST_CONTENTFUL_PAINT,
};
const addSpanPerformancePaintEvents = (span) => {
    const performancePaintTiming = otperformance.getEntriesByType?.('paint');
    if (performancePaintTiming) {
        performancePaintTiming.forEach(({ name, startTime }) => {
            if (hasKey(performancePaintNames, name)) {
                span.addEvent(performancePaintNames[name], startTime);
            }
        });
    }
};
//# sourceMappingURL=utils.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+instrumentation-document-load@0.67.0+4294d048f15f6cd7/node_modules/@opentelemetry/instrumentation-document-load/build/esm/instrumentation.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */






/** @knipignore */


/**
 * This class represents a document load plugin
 */
class DocumentLoadInstrumentation extends InstrumentationBase {
    component = 'document-load';
    version = '1';
    moduleName = this.component;
    constructor(config = {}) {
        super(PACKAGE_NAME, PACKAGE_VERSION, config);
    }
    init() { }
    /**
     * callback to be executed when page is loaded
     */
    _onDocumentLoaded() {
        // Timeout is needed as load event doesn't have yet the performance metrics for loadEnd.
        // Support for event "loadend" is very limited and cannot be used
        window.setTimeout(() => {
            this._collectPerformance();
        });
    }
    /**
     * Adds spans for all resources
     * @param rootSpan
     */
    _addResourcesSpans(rootSpan) {
        const resources = otperformance.getEntriesByType?.('resource');
        if (resources) {
            resources.forEach(resource => {
                this._initResourceSpan(resource, rootSpan);
            });
        }
    }
    /**
     * Collects information about performance and creates appropriate spans
     */
    _collectPerformance() {
        const metaElement = Array.from(document.getElementsByTagName('meta')).find(e => e.getAttribute('name') === TRACE_PARENT_HEADER);
        const entries = getPerformanceNavigationEntries();
        const traceparent = (metaElement && metaElement.content) || '';
        context_api_context.with(propagation.extract(context_context/* ROOT_CONTEXT */.l, { traceparent }), () => {
            const rootSpan = this._startSpan(AttributeNames.DOCUMENT_LOAD, PerformanceTimingNames.FETCH_START, entries);
            if (!rootSpan) {
                return;
            }
            context_api_context.with(trace_api/* trace */.u.setSpan(context_api_context.active(), rootSpan), () => {
                const fetchSpan = this._startSpan(AttributeNames.DOCUMENT_FETCH, PerformanceTimingNames.FETCH_START, entries);
                if (fetchSpan) {
                    fetchSpan.setAttribute(ATTR_URL_FULL, location.href);
                    context_api_context.with(trace_api/* trace */.u.setSpan(context_api_context.active(), fetchSpan), () => {
                        addSpanNetworkEvents(fetchSpan, entries, this.getConfig().ignoreNetworkEvents);
                        this._addCustomAttributesOnSpan(fetchSpan, this.getConfig().applyCustomAttributesOnSpan?.documentFetch);
                        this._endSpan(fetchSpan, PerformanceTimingNames.RESPONSE_END, entries);
                    });
                }
            });
            rootSpan.setAttribute(ATTR_URL_FULL, location.href);
            rootSpan.setAttribute(ATTR_USER_AGENT_ORIGINAL, navigator.userAgent);
            this._addResourcesSpans(rootSpan);
            if (!this.getConfig().ignoreNetworkEvents) {
                addSpanNetworkEvent(rootSpan, PerformanceTimingNames.FETCH_START, entries);
                addSpanNetworkEvent(rootSpan, PerformanceTimingNames.UNLOAD_EVENT_START, entries);
                addSpanNetworkEvent(rootSpan, PerformanceTimingNames.UNLOAD_EVENT_END, entries);
                addSpanNetworkEvent(rootSpan, PerformanceTimingNames.DOM_INTERACTIVE, entries);
                addSpanNetworkEvent(rootSpan, PerformanceTimingNames.DOM_CONTENT_LOADED_EVENT_START, entries);
                addSpanNetworkEvent(rootSpan, PerformanceTimingNames.DOM_CONTENT_LOADED_EVENT_END, entries);
                addSpanNetworkEvent(rootSpan, PerformanceTimingNames.DOM_COMPLETE, entries);
                addSpanNetworkEvent(rootSpan, PerformanceTimingNames.LOAD_EVENT_START, entries);
                addSpanNetworkEvent(rootSpan, PerformanceTimingNames.LOAD_EVENT_END, entries);
            }
            if (!this.getConfig().ignorePerformancePaintEvents) {
                addSpanPerformancePaintEvents(rootSpan);
            }
            this._addCustomAttributesOnSpan(rootSpan, this.getConfig().applyCustomAttributesOnSpan?.documentLoad);
            this._endSpan(rootSpan, PerformanceTimingNames.LOAD_EVENT_END, entries);
        });
    }
    /**
     * Helper function for ending span
     * @param span
     * @param performanceName name of performance entry for time end
     * @param entries
     */
    _endSpan(span, performanceName, entries) {
        // span can be undefined when entries are missing the certain performance - the span will not be created
        if (span) {
            if (hasKey(entries, performanceName)) {
                span.end(entries[performanceName]);
            }
            else {
                // just end span
                span.end();
            }
        }
    }
    /**
     * Creates and ends a span with network information about resource added as timed events
     * @param resource
     * @param parentSpan
     */
    _initResourceSpan(resource, parentSpan) {
        const span = this._startSpan(AttributeNames.RESOURCE_FETCH, PerformanceTimingNames.FETCH_START, resource, parentSpan);
        if (span) {
            span.setAttribute(ATTR_URL_FULL, resource.name);
            addSpanNetworkEvents(span, resource, this.getConfig().ignoreNetworkEvents);
            this._addCustomAttributesOnResourceSpan(span, resource, this.getConfig().applyCustomAttributesOnSpan?.resourceFetch);
            this._endSpan(span, PerformanceTimingNames.RESPONSE_END, resource);
        }
    }
    /**
     * Helper function for starting a span
     * @param spanName name of span
     * @param performanceName name of performance entry for time start
     * @param entries
     * @param parentSpan
     */
    _startSpan(spanName, performanceName, entries, parentSpan) {
        if (hasKey(entries, performanceName) &&
            typeof entries[performanceName] === 'number') {
            const span = this.tracer.startSpan(spanName, {
                startTime: entries[performanceName],
            }, parentSpan ? trace_api/* trace */.u.setSpan(context_api_context.active(), parentSpan) : undefined);
            return span;
        }
        return undefined;
    }
    /**
     * executes callback {_onDocumentLoaded} when the page is loaded
     */
    _waitForPageLoad() {
        if (window.document.readyState === 'complete') {
            this._onDocumentLoaded();
        }
        else {
            this._onDocumentLoaded = this._onDocumentLoaded.bind(this);
            window.addEventListener('load', this._onDocumentLoaded);
        }
    }
    /**
     * adds custom attributes to root span if configured
     */
    _addCustomAttributesOnSpan(span, applyCustomAttributesOnSpan) {
        if (applyCustomAttributesOnSpan) {
            safeExecuteInTheMiddle(() => applyCustomAttributesOnSpan(span), error => {
                if (!error) {
                    return;
                }
                this._diag.error('addCustomAttributesOnSpan', error);
            }, true);
        }
    }
    /**
     * adds custom attributes to span if configured
     */
    _addCustomAttributesOnResourceSpan(span, resource, applyCustomAttributesOnSpan) {
        if (applyCustomAttributesOnSpan) {
            safeExecuteInTheMiddle(() => applyCustomAttributesOnSpan(span, resource), error => {
                if (!error) {
                    return;
                }
                this._diag.error('addCustomAttributesOnResourceSpan', error);
            }, true);
        }
    }
    /**
     * implements enable function
     */
    enable() {
        // remove previously attached load to avoid adding the same event twice
        // in case of multiple enable calling.
        window.removeEventListener('load', this._onDocumentLoaded);
        this._waitForPageLoad();
    }
    /**
     * implements disable function
     */
    disable() {
        window.removeEventListener('load', this._onDocumentLoaded);
    }
}
//# sourceMappingURL=instrumentation.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+instrumentation-document-load@0.67.0+4294d048f15f6cd7/node_modules/@opentelemetry/instrumentation-document-load/build/esm/index.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */


//# sourceMappingURL=index.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api@1.9.1/node_modules/@opentelemetry/api/build/esm/trace/span_kind.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
/**
 * @since 1.0.0
 */
var SpanKind;
(function (SpanKind) {
    /** Default value. Indicates that the span is used internally. */
    SpanKind[SpanKind["INTERNAL"] = 0] = "INTERNAL";
    /**
     * Indicates that the span covers server-side handling of an RPC or other
     * remote request.
     */
    SpanKind[SpanKind["SERVER"] = 1] = "SERVER";
    /**
     * Indicates that the span covers the client-side wrapper around an RPC or
     * other remote request.
     */
    SpanKind[SpanKind["CLIENT"] = 2] = "CLIENT";
    /**
     * Indicates that the span describes producer sending a message to a
     * broker. Unlike client and server, there is no direct critical path latency
     * relationship between producer and consumer spans.
     */
    SpanKind[SpanKind["PRODUCER"] = 3] = "PRODUCER";
    /**
     * Indicates that the span describes consumer receiving a message from a
     * broker. Unlike client and server, there is no direct critical path latency
     * relationship between producer and consumer spans.
     */
    SpanKind[SpanKind["CONSUMER"] = 4] = "CONSUMER";
})(SpanKind || (SpanKind = {}));
//# sourceMappingURL=span_kind.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api@1.9.1/node_modules/@opentelemetry/api/build/esm/trace/status.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
/**
 * An enumeration of status codes.
 *
 * @since 1.0.0
 */
var SpanStatusCode;
(function (SpanStatusCode) {
    /**
     * The default status.
     */
    SpanStatusCode[SpanStatusCode["UNSET"] = 0] = "UNSET";
    /**
     * The operation has been validated by an Application developer or
     * Operator to have completed successfully.
     */
    SpanStatusCode[SpanStatusCode["OK"] = 1] = "OK";
    /**
     * The operation contains an error.
     */
    SpanStatusCode[SpanStatusCode["ERROR"] = 2] = "ERROR";
})(SpanStatusCode || (SpanStatusCode = {}));
//# sourceMappingURL=status.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+instrumentation-fetch@0.222.0+4294d048f15f6cd7/node_modules/@opentelemetry/instrumentation-fetch/build/esm/semconv.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
/*
 * This file contains a copy of unstable semantic convention definitions
 * used by this package.
 * @see https://github.com/open-telemetry/opentelemetry-js/tree/main/semantic-conventions#unstable-semconv
 */
/**
 * The size of the request payload body in bytes. This is the number of bytes transferred excluding headers and is often, but not always, present as the [Content-Length](https://www.rfc-editor.org/rfc/rfc9110.html#field.content-length) header. For requests using transport encoding, this should be the compressed size.
 *
 * @example 3495
 *
 * @experimental This attribute is experimental and is subject to breaking changes in minor releases of `@opentelemetry/semantic-conventions`.
 */
const ATTR_HTTP_REQUEST_BODY_SIZE = 'http.request.body.size';
//# sourceMappingURL=semconv.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+core@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/core/build/esm/platform/node/environment.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */


/**
 * Retrieves a number from an environment variable.
 * - Returns `undefined` if the environment variable is empty, unset, contains only whitespace, or is not a number.
 * - Returns a number in all other cases.
 *
 * @param {string} key - The name of the environment variable to retrieve.
 * @returns {number | undefined} - The number value or `undefined`.
 */
function getNumberFromEnv(key) {
    const raw = process.env[key];
    if (raw == null || raw.trim() === '') {
        return undefined;
    }
    const value = Number(raw);
    if (isNaN(value)) {
        diag_api_diag.warn(`Unknown value ${(0,external_util_.inspect)(raw)} for ${key}, expected a number, using defaults`);
        return undefined;
    }
    return value;
}
/**
 * Retrieves a string from an environment variable.
 * - Returns `undefined` if the environment variable is empty, unset, or contains only whitespace.
 *
 * @param {string} key - The name of the environment variable to retrieve.
 * @returns {string | undefined} - The string value or `undefined`.
 */
function getStringFromEnv(key) {
    const raw = process.env[key];
    if (raw == null || raw.trim() === '') {
        return undefined;
    }
    return raw;
}
/**
 * Retrieves a boolean value from an environment variable.
 * - Trims leading and trailing whitespace and ignores casing.
 * - Returns `false` if the environment variable is empty, unset, or contains only whitespace.
 * - Returns `false` for strings that cannot be mapped to a boolean.
 *
 * @param {string} key - The name of the environment variable to retrieve.
 * @returns {boolean} - The boolean value or `false` if the environment variable is unset empty, unset, or contains only whitespace.
 */
function getBooleanFromEnv(key) {
    const raw = process.env[key]?.trim().toLowerCase();
    if (raw == null || raw === '') {
        // NOTE: falling back to `false` instead of `undefined` as required by the specification.
        // If you have a use-case that requires `undefined`, consider using `getStringFromEnv()` and applying the necessary
        // normalizations in the consuming code.
        return false;
    }
    if (raw === 'true') {
        return true;
    }
    else if (raw === 'false') {
        return false;
    }
    else {
        diag.warn(`Unknown value ${inspect(raw)} for ${key}, expected 'true' or 'false', falling back to 'false' (default)`);
        return false;
    }
}
/**
 * Retrieves a list of strings from an environment variable.
 * - Uses ',' as the delimiter.
 * - Trims leading and trailing whitespace from each entry.
 * - Excludes empty entries.
 * - Returns `undefined` if the environment variable is empty or contains only whitespace.
 * - Returns an empty array if all entries are empty or whitespace.
 *
 * @param {string} key - The name of the environment variable to retrieve.
 * @returns {string[] | undefined} - The list of strings or `undefined`.
 */
function getStringListFromEnv(key) {
    return getStringFromEnv(key)
        ?.split(',')
        .map(v => v.trim())
        .filter(s => s !== '');
}
//# sourceMappingURL=environment.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+instrumentation-fetch@0.222.0+4294d048f15f6cd7/node_modules/@opentelemetry/instrumentation-fetch/build/esm/utils.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
// Much of the logic here overlaps with the same utils file in opentelemetry-instrumentation-xml-http-request
// These may be unified in the future.


const DIAG_LOGGER = diag_api_diag.createComponentLogger({
    namespace: '@opentelemetry/opentelemetry-instrumentation-fetch/utils',
});
/**
 * Helper function to determine payload content length for fetch requests
 *
 * The fetch API is kinda messy: there are a couple of ways the body can be passed in.
 *
 * In all cases, the body param can be some variation of ReadableStream,
 * and ReadableStreams can only be read once! We want to avoid consuming the body here,
 * because that would mean that the body never gets sent with the actual fetch request.
 *
 * Either the first arg is a Request object, which can be cloned
 *   so we can clone that object and read the body of the clone
 *   without disturbing the original argument
 *   However, reading the body here can only be done async; the body() method returns a promise
 *   this means this entire function has to return a promise
 *
 * OR the first arg is a url/string
 *   in which case the second arg has type RequestInit
 *   RequestInit is NOT cloneable, but RequestInit.body is writable
 *   so we can chain it into ReadableStream.pipeThrough()
 *
 *   ReadableStream.pipeThrough() lets us process a stream and returns a new stream
 *   So we can measure the body length as it passes through the pie, but need to attach
 *   the new stream to the original request
 *   so that the browser still has access to the body.
 *
 * @param body
 * @returns promise that resolves to the content length of the body
 */
function getFetchBodyLength(...args) {
    if (args[0] instanceof URL || typeof args[0] === 'string') {
        const requestInit = args[1];
        if (!requestInit?.body) {
            return Promise.resolve();
        }
        if (requestInit.body instanceof ReadableStream) {
            const { body, length } = _getBodyNonDestructively(requestInit.body);
            requestInit.body = body;
            return length;
        }
        else {
            return Promise.resolve(getXHRBodyLength(requestInit.body));
        }
    }
    else {
        const info = args[0];
        if (!info?.body) {
            return Promise.resolve();
        }
        return info
            .clone()
            .text()
            .then(t => getByteLength(t));
    }
}
function _getBodyNonDestructively(body) {
    // can't read a ReadableStream without destroying it
    // but we CAN pipe it through and return a new ReadableStream
    // some (older) platforms don't expose the pipeThrough method and in that scenario, we're out of luck;
    //   there's no way to read the stream without consuming it.
    if (!body.pipeThrough) {
        DIAG_LOGGER.warn('Platform has ReadableStream but not pipeThrough!');
        return {
            body,
            length: Promise.resolve(undefined),
        };
    }
    let length = 0;
    let resolveLength;
    const lengthPromise = new Promise(resolve => {
        resolveLength = resolve;
    });
    const transform = new TransformStream({
        start() { },
        async transform(chunk, controller) {
            const bytearray = (await chunk);
            length += bytearray.byteLength;
            controller.enqueue(chunk);
        },
        flush() {
            resolveLength(length);
        },
    });
    return {
        body: body.pipeThrough(transform),
        length: lengthPromise,
    };
}
function isDocument(value) {
    return typeof Document !== 'undefined' && value instanceof Document;
}
/**
 * Helper function to determine payload content length for XHR requests
 * @param body
 * @returns content length
 */
function getXHRBodyLength(body) {
    if (isDocument(body)) {
        return new XMLSerializer().serializeToString(document).length;
    }
    // XMLHttpRequestBodyInit expands to the following:
    if (typeof body === 'string') {
        return getByteLength(body);
    }
    if (body instanceof Blob) {
        return body.size;
    }
    if (body instanceof FormData) {
        return getFormDataSize(body);
    }
    if (body instanceof URLSearchParams) {
        return getByteLength(body.toString());
    }
    // ArrayBuffer | ArrayBufferView
    if (body.byteLength !== undefined) {
        return body.byteLength;
    }
    DIAG_LOGGER.warn('unknown body type');
    return undefined;
}
const TEXT_ENCODER = new TextEncoder();
function getByteLength(s) {
    return TEXT_ENCODER.encode(s).byteLength;
}
function getFormDataSize(formData) {
    let size = 0;
    for (const [key, value] of formData.entries()) {
        size += key.length;
        if (value instanceof Blob) {
            size += value.size;
        }
        else {
            size += value.length;
        }
    }
    return size;
}
/**
 * Normalize an HTTP request method string per `http.request.method` spec
 * https://github.com/open-telemetry/semantic-conventions/blob/main/docs/http/http-spans.md#http-client-span
 */
function normalizeHttpRequestMethod(method) {
    const knownMethods = getKnownMethods();
    const methUpper = method.toUpperCase();
    if (methUpper in knownMethods) {
        return methUpper;
    }
    else {
        return '_OTHER';
    }
}
const DEFAULT_KNOWN_METHODS = {
    CONNECT: true,
    DELETE: true,
    GET: true,
    HEAD: true,
    OPTIONS: true,
    PATCH: true,
    POST: true,
    PUT: true,
    TRACE: true,
    // QUERY from https://datatracker.ietf.org/doc/draft-ietf-httpbis-safe-method-w-body/
    QUERY: true,
};
let knownMethods;
function getKnownMethods() {
    if (knownMethods === undefined) {
        const cfgMethods = getStringListFromEnv('OTEL_INSTRUMENTATION_HTTP_KNOWN_METHODS');
        if (cfgMethods && cfgMethods.length > 0) {
            knownMethods = {};
            cfgMethods.forEach(m => {
                knownMethods[m] = true;
            });
        }
        else {
            knownMethods = DEFAULT_KNOWN_METHODS;
        }
    }
    return knownMethods;
}
const HTTP_PORT_FROM_PROTOCOL = {
    'https:': '443',
    'http:': '80',
};
function serverPortFromUrl(url) {
    const serverPort = Number(url.port || HTTP_PORT_FROM_PROTOCOL[url.protocol]);
    // Guard with `if (serverPort)` because `Number('') === 0`.
    if (serverPort && !isNaN(serverPort)) {
        return serverPort;
    }
    else {
        return undefined;
    }
}
//# sourceMappingURL=utils.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+instrumentation-fetch@0.222.0+4294d048f15f6cd7/node_modules/@opentelemetry/instrumentation-fetch/build/esm/version.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
// this is autogenerated file, see scripts/version-update.js
const version_VERSION = '0.222.0';
//# sourceMappingURL=version.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+instrumentation-fetch@0.222.0+4294d048f15f6cd7/node_modules/@opentelemetry/instrumentation-fetch/build/esm/fetch.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */








// how long to wait for observer to collect information about resources
// this is needed as event "load" is called before observer
// hard to say how long it should really wait, seems like 300ms is
// safe enough
const OBSERVER_WAIT_TIME_MS = 300;
const hasBrowserPerformanceAPI = typeof PerformanceObserver !== 'undefined';
/**
 * This class represents a fetch plugin for auto instrumentation
 */
class FetchInstrumentation extends InstrumentationBase {
    component = 'fetch';
    version = version_VERSION;
    moduleName = this.component;
    _usedResources = new WeakSet();
    _tasksCount = 0;
    constructor(config = {}) {
        super('@opentelemetry/instrumentation-fetch', version_VERSION, config);
    }
    init() { }
    /**
     * Add cors pre flight child span
     * @param span
     * @param corsPreFlightRequest
     */
    _addChildSpan(span, corsPreFlightRequest) {
        const childSpan = this.tracer.startSpan('CORS Preflight', {
            startTime: corsPreFlightRequest[PerformanceTimingNames.FETCH_START],
        }, trace_api/* trace */.u.setSpan(context_api_context.active(), span));
        addSpanNetworkEvents(childSpan, corsPreFlightRequest, this.getConfig().ignoreNetworkEvents, undefined, true);
        childSpan.end(corsPreFlightRequest[PerformanceTimingNames.RESPONSE_END]);
    }
    /**
     * Adds more attributes to span just before ending it
     * @param span
     * @param response
     */
    _addFinalSpanAttributes(span, response) {
        const parsedUrl = parseUrl(response.url);
        span.setAttribute(ATTR_HTTP_RESPONSE_STATUS_CODE, response.status);
        // TODO: Set server.{address,port} at span creation for sampling decisions
        // (a "SHOULD" requirement in semconv).
        span.setAttribute(ATTR_SERVER_ADDRESS, parsedUrl.hostname);
        const serverPort = serverPortFromUrl(parsedUrl);
        if (serverPort) {
            span.setAttribute(ATTR_SERVER_PORT, serverPort);
        }
    }
    /**
     * Add headers
     * @param options
     * @param spanUrl
     */
    _addHeaders(options, spanUrl) {
        if (!shouldPropagateTraceHeaders(spanUrl, this.getConfig().propagateTraceHeaderCorsUrls)) {
            const headers = {};
            propagation.inject(context_api_context.active(), headers);
            if (Object.keys(headers).length > 0) {
                this._diag.debug('headers inject skipped due to CORS policy');
            }
            return;
        }
        if (options instanceof Request) {
            // This mutates `Request.headers` in-place, because it is read-only
            // (per https://developer.mozilla.org/en-US/docs/Web/API/Request/headers),
            // so we cannot (easily) replace it.
            propagation.inject(context_api_context.active(), options.headers, {
                set: (h, k, v) => h.set(k, typeof v === 'string' ? v : String(v)),
            });
        }
        else {
            // Otherwise, create a new Headers to avoid mutating the caller's
            // possibly re-used headers.
            const headers = new Headers(options.headers);
            propagation.inject(context_api_context.active(), headers, {
                set: (h, k, v) => h.set(k, typeof v === 'string' ? v : String(v)),
            });
            options.headers = headers;
        }
    }
    /**
     * Clears the resource timings and all resources assigned with spans
     *     when {@link FetchPluginConfig.clearTimingResources} is
     *     set to true (default false)
     * @private
     */
    _clearResources() {
        if (this._tasksCount === 0 && this.getConfig().clearTimingResources) {
            performance.clearResourceTimings();
            this._usedResources = new WeakSet();
        }
    }
    /**
     * Creates a new span
     * @param url
     * @param options
     */
    _createSpan(url, options = {}) {
        if (isUrlIgnored(url, this.getConfig().ignoreUrls)) {
            this._diag.debug('ignoring span as url matches ignored url');
            return;
        }
        const attributes = {};
        const origMethod = options.method;
        const normMethod = normalizeHttpRequestMethod(options.method || 'GET');
        const name = normMethod;
        attributes[ATTR_HTTP_REQUEST_METHOD] = normMethod;
        if (normMethod !== origMethod) {
            attributes[ATTR_HTTP_REQUEST_METHOD_ORIGINAL] = origMethod;
        }
        attributes[ATTR_URL_FULL] = url;
        return this.tracer.startSpan(name, {
            kind: SpanKind.CLIENT,
            attributes,
        });
    }
    /**
     * Finds appropriate resource and add network events to the span
     * @param span
     * @param resourcesObserver
     * @param endTime
     */
    _findResourceAndAddNetworkEvents(span, resourcesObserver, endTime) {
        let resources = resourcesObserver.entries;
        if (!resources.length) {
            if (!performance.getEntriesByType) {
                return;
            }
            // fallback - either Observer is not available or it took longer
            // then OBSERVER_WAIT_TIME_MS and observer didn't collect enough
            // information
            resources = performance.getEntriesByType('resource');
        }
        const resource = getResource(resourcesObserver.spanUrl, resourcesObserver.startTime, endTime, resources, this._usedResources, 'fetch');
        if (resource.mainRequest) {
            const mainRequest = resource.mainRequest;
            this._markResourceAsUsed(mainRequest);
            const corsPreFlightRequest = resource.corsPreFlightRequest;
            if (corsPreFlightRequest) {
                this._addChildSpan(span, corsPreFlightRequest);
                this._markResourceAsUsed(corsPreFlightRequest);
            }
            addSpanNetworkEvents(span, mainRequest, this.getConfig().ignoreNetworkEvents, undefined, true);
        }
    }
    /**
     * Marks certain [resource]{@link PerformanceResourceTiming} when information
     * from this is used to add events to span.
     * This is done to avoid reusing the same resource again for next span
     * @param resource
     */
    _markResourceAsUsed(resource) {
        this._usedResources.add(resource);
    }
    /**
     * Finish span, add attributes, network events etc.
     * @param span
     * @param spanData
     * @param response
     */
    _endSpan(span, spanData, response) {
        const endTime = millisToHrTime(Date.now());
        const performanceEndTime = hrTime();
        this._addFinalSpanAttributes(span, response);
        // https://github.com/open-telemetry/semantic-conventions/blob/main/docs/http/http-spans.md#status
        if (response.status >= 400) {
            span.setStatus({ code: SpanStatusCode.ERROR });
            span.setAttribute(ATTR_ERROR_TYPE, String(response.status));
        }
        setTimeout(() => {
            spanData.observer?.disconnect();
            this._findResourceAndAddNetworkEvents(span, spanData, performanceEndTime);
            this._tasksCount--;
            this._clearResources();
            span.end(endTime);
        }, OBSERVER_WAIT_TIME_MS);
    }
    /**
     * Patches the constructor of fetch
     */
    _patchConstructor() {
        return original => {
            const plugin = this;
            return function patchConstructor(...args) {
                if (!plugin._isEnabled) {
                    return original.apply(this, args);
                }
                const self = this;
                const url = parseUrl(args[0] instanceof Request ? args[0].url : String(args[0])).href;
                // Per the Fetch spec, when fetch() is called with a Request object
                // and a separate init object, the init properties override the
                // Request's properties. Merge them into a new Request so that
                // downstream consumers (hooks, header injection, the actual fetch
                // call) see the correct final values.
                // See: https://developer.mozilla.org/en-US/docs/Web/API/Request/Request#parameters
                let options;
                if (args[0] instanceof Request) {
                    options = args[1] != null ? new Request(args[0], args[1]) : args[0];
                }
                else {
                    options = args[1] || {};
                }
                const createdSpan = plugin._createSpan(url, options);
                if (!createdSpan) {
                    return original.apply(this, args);
                }
                const spanData = plugin._prepareSpanData(url);
                if (plugin.getConfig().measureRequestSize) {
                    getFetchBodyLength(...args)
                        .then(bodyLength => {
                        if (!bodyLength)
                            return;
                        createdSpan.setAttribute(ATTR_HTTP_REQUEST_BODY_SIZE, bodyLength);
                    })
                        .catch(error => {
                        plugin._diag.warn('getFetchBodyLength', error);
                    });
                }
                function endSpanOnError(span, error) {
                    plugin._applyAttributesAfterFetch(span, options, error);
                    plugin._endSpan(span, spanData, {
                        status: error.status || 0,
                        statusText: error.message,
                        url,
                    });
                }
                function endSpanOnSuccess(span, response) {
                    plugin._applyAttributesAfterFetch(span, options, response);
                    if (response.status >= 200 && response.status < 400) {
                        plugin._endSpan(span, spanData, response);
                    }
                    else {
                        plugin._endSpan(span, spanData, {
                            status: response.status,
                            statusText: response.statusText,
                            url,
                        });
                    }
                }
                function onSuccess(span, response) {
                    try {
                        // Clone the response and eagerly consume the clone to detect
                        // when the body transfer completes.  The original response is
                        // returned to the caller untouched so that it passes internal
                        // brand-checks required by APIs such as
                        // WebAssembly.compileStreaming.
                        // TODO: Switch to a consumer-driven model and drop `resClone`.
                        // Keeping eager consumption here to preserve current behavior and avoid breaking existing tests.
                        // Context: discussion in PR #5894 → https://github.com/open-telemetry/opentelemetry-js/pull/5894
                        const resClone = response.clone();
                        const body = resClone.body;
                        if (body) {
                            const reader = body.getReader();
                            const read = () => {
                                reader.read().then(({ done }) => {
                                    if (done) {
                                        endSpanOnSuccess(span, response);
                                    }
                                    else {
                                        read();
                                    }
                                }, error => {
                                    endSpanOnError(span, error);
                                });
                            };
                            read();
                        }
                        else {
                            // some older browsers don't have .body implemented
                            endSpanOnSuccess(span, response);
                        }
                    }
                    catch (error) {
                        // Setup failed (e.g. clone() or getReader() threw).
                        // End the span and clean up so _tasksCount doesn't leak.
                        plugin._diag.error('Failed to read fetch response body', error);
                        plugin._endSpan(span, spanData, {
                            status: 0,
                            url,
                        });
                    }
                    return response;
                }
                function onError(span, error) {
                    try {
                        endSpanOnError(span, error);
                    }
                    catch (e) {
                        // endSpanOnError failed — fall back to ending the span
                        // directly so _tasksCount doesn't leak.
                        plugin._diag.error('Failed to end span on fetch error', e);
                        plugin._endSpan(span, spanData, {
                            status: error.status || 0,
                            url,
                        });
                    }
                    throw error;
                }
                return context_api_context.with(trace_api/* trace */.u.setSpan(context_api_context.active(), createdSpan), () => {
                    // Call request hook before injection so hooks cannot tamper with propagation headers.
                    // Also, this means the hook will see `options.headers` in the same type as passed in,
                    // rather than as a `Headers` instance set by `_addHeaders()`.
                    plugin._callRequestHook(createdSpan, options);
                    plugin._addHeaders(options, url);
                    plugin._tasksCount++;
                    return original
                        .apply(self, options instanceof Request ? [options] : [url, options])
                        .then(onSuccess.bind(self, createdSpan), onError.bind(self, createdSpan));
                });
            };
        };
    }
    _applyAttributesAfterFetch(span, request, result) {
        const applyCustomAttributesOnSpan = this.getConfig().applyCustomAttributesOnSpan;
        if (applyCustomAttributesOnSpan) {
            safeExecuteInTheMiddle(() => applyCustomAttributesOnSpan(span, request, result), error => {
                if (!error) {
                    return;
                }
                this._diag.error('applyCustomAttributesOnSpan', error);
            }, true);
        }
    }
    _callRequestHook(span, request) {
        const requestHook = this.getConfig().requestHook;
        if (requestHook) {
            safeExecuteInTheMiddle(() => requestHook(span, request), error => {
                if (!error) {
                    return;
                }
                this._diag.error('requestHook', error);
            }, true);
        }
    }
    /**
     * Prepares a span data - needed later for matching appropriate network
     *     resources
     * @param spanUrl
     */
    _prepareSpanData(spanUrl) {
        const startTime = hrTime();
        const entries = [];
        if (typeof PerformanceObserver !== 'function') {
            return { entries, startTime, spanUrl };
        }
        const observer = new PerformanceObserver(list => {
            const perfObsEntries = list.getEntries();
            perfObsEntries.forEach(entry => {
                if (entry.initiatorType === 'fetch' && entry.name === spanUrl) {
                    entries.push(entry);
                }
            });
        });
        observer.observe({
            entryTypes: ['resource'],
        });
        return { entries, observer, startTime, spanUrl };
    }
    /**
     * implements enable function
     */
    enable() {
        if (!hasBrowserPerformanceAPI) {
            this._diag.warn('this instrumentation is intended for web usage only, it does not instrument server-side fetch()');
            return;
        }
        if (this._isEnabled) {
            return;
        }
        if (this._isFetchPatched) {
            this._diag.debug('fetch constructor already patched');
            this._isEnabled = true;
            return;
        }
        try {
            // `_wrap` throws if a third-party script has locked globalThis.fetch via
            // Object.defineProperty(window, 'fetch', { writable: false, ... }).
            this._wrap(globalThis, 'fetch', this._patchConstructor());
            this._isFetchPatched = true;
            this._isEnabled = true;
        }
        catch (err) {
            this._diag.warn('Failed to patch globalThis.fetch; instrumentation will not be enabled. ' +
                'Another script may have locked globalThis.fetch via Object.defineProperty.', err);
        }
    }
    /**
     * deactivates fetch instrumentation
     */
    disable() {
        if (!hasBrowserPerformanceAPI) {
            return;
        }
        if (!this._isEnabled) {
            return;
        }
        this._isEnabled = false;
        this._usedResources = new WeakSet();
    }
}
//# sourceMappingURL=fetch.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+otlp-exporter-base@0.222.0+e40b0dfdd726a224/node_modules/@opentelemetry/otlp-exporter-base/build/esm/OTLPExporterBase.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
class OTLPExporterBase {
    _delegate;
    constructor(delegate) {
        this._delegate = delegate;
    }
    /**
     * Export items.
     * @param items
     * @param resultCallback
     */
    export(items, resultCallback) {
        this._delegate.export(items, resultCallback);
    }
    forceFlush() {
        return this._delegate.forceFlush();
    }
    shutdown() {
        return this._delegate.shutdown();
    }
    setMetrics(metrics) {
        this._delegate.setMetrics(metrics);
    }
}
//# sourceMappingURL=OTLPExporterBase.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+otlp-transformer@0.222.0+e40b0dfdd726a224/node_modules/@opentelemetry/otlp-transformer/build/esm/common/internal.js
function createResource(resource, encoder) {
    const result = {
        attributes: toAttributes(resource.attributes, encoder),
        droppedAttributesCount: 0,
    };
    const schemaUrl = resource.schemaUrl;
    if (schemaUrl && schemaUrl !== '')
        result.schemaUrl = schemaUrl;
    return result;
}
function createInstrumentationScope(scope, encoder) {
    const result = {
        name: scope.name,
        version: scope.version,
    };
    if (scope.attributes && Object.keys(scope.attributes).length > 0) {
        result.attributes = toAttributes(scope.attributes, encoder);
        result.droppedAttributesCount = scope.droppedAttributesCount ?? 0;
    }
    return result;
}
function toAttributes(attributes, encoder) {
    return Object.keys(attributes).map(key => toKeyValue(key, attributes[key], encoder));
}
function toKeyValue(key, value, encoder) {
    return {
        key: key,
        value: toAnyValue(value, encoder),
    };
}
function toAnyValue(value, encoder) {
    const t = typeof value;
    if (t === 'string')
        return { stringValue: value };
    if (t === 'number') {
        if (!Number.isInteger(value))
            return { doubleValue: value };
        return { intValue: value };
    }
    if (t === 'boolean')
        return { boolValue: value };
    if (value instanceof Uint8Array)
        return { bytesValue: encoder.encodeUint8Array(value) };
    if (Array.isArray(value)) {
        const values = new Array(value.length);
        for (let i = 0; i < value.length; i++) {
            values[i] = toAnyValue(value[i], encoder);
        }
        return { arrayValue: { values } };
    }
    if (t === 'object' && value != null) {
        const keys = Object.keys(value);
        const values = new Array(keys.length);
        for (let i = 0; i < keys.length; i++) {
            values[i] = {
                key: keys[i],
                value: toAnyValue(value[keys[i]], encoder),
            };
        }
        return { kvlistValue: { values } };
    }
    return {};
}
//# sourceMappingURL=internal.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+otlp-transformer@0.222.0+e40b0dfdd726a224/node_modules/@opentelemetry/otlp-transformer/build/esm/trace/internal.js

// Span flags constants matching the OTLP specification
const SPAN_FLAGS_CONTEXT_HAS_IS_REMOTE_MASK = 0x100;
const SPAN_FLAGS_CONTEXT_IS_REMOTE_MASK = 0x200;
/**
 * Builds the 32-bit span flags value combining the low 8-bit W3C TraceFlags
 * with the HAS_IS_REMOTE and IS_REMOTE bits according to the OTLP spec.
 */
function buildSpanFlagsFrom(traceFlags, isRemote) {
    // low 8 bits are W3C TraceFlags (e.g., sampled)
    let flags = (traceFlags & 0xff) | SPAN_FLAGS_CONTEXT_HAS_IS_REMOTE_MASK;
    if (isRemote) {
        flags |= SPAN_FLAGS_CONTEXT_IS_REMOTE_MASK;
    }
    return flags;
}
function sdkSpanToOtlpSpan(span, encoder) {
    const ctx = span.spanContext();
    const status = span.status;
    const parentSpanId = span.parentSpanContext?.spanId
        ? encoder.encodeSpanContext(span.parentSpanContext?.spanId)
        : undefined;
    return {
        traceId: encoder.encodeSpanContext(ctx.traceId),
        spanId: encoder.encodeSpanContext(ctx.spanId),
        parentSpanId: parentSpanId,
        traceState: ctx.traceState?.serialize(),
        name: span.name,
        // Span kind is offset by 1 because the API does not define a value for unset
        kind: span.kind == null ? 0 : span.kind + 1,
        startTimeUnixNano: encoder.encodeHrTime(span.startTime),
        endTimeUnixNano: encoder.encodeHrTime(span.endTime),
        attributes: toAttributes(span.attributes, encoder),
        droppedAttributesCount: span.droppedAttributesCount,
        events: span.events.map(event => toOtlpSpanEvent(event, encoder)),
        droppedEventsCount: span.droppedEventsCount,
        status: {
            // API and proto enums share the same values
            code: status.code,
            message: status.message,
        },
        links: span.links.map(link => toOtlpLink(link, encoder)),
        droppedLinksCount: span.droppedLinksCount,
        flags: buildSpanFlagsFrom(ctx.traceFlags, span.parentSpanContext?.isRemote),
    };
}
function toOtlpLink(link, encoder) {
    return {
        attributes: link.attributes ? toAttributes(link.attributes, encoder) : [],
        spanId: encoder.encodeSpanContext(link.context.spanId),
        traceId: encoder.encodeSpanContext(link.context.traceId),
        traceState: link.context.traceState?.serialize(),
        droppedAttributesCount: link.droppedAttributesCount || 0,
        flags: buildSpanFlagsFrom(link.context.traceFlags, link.context.isRemote),
    };
}
function toOtlpSpanEvent(timedEvent, encoder) {
    return {
        attributes: timedEvent.attributes
            ? toAttributes(timedEvent.attributes, encoder)
            : [],
        name: timedEvent.name,
        timeUnixNano: encoder.encodeHrTime(timedEvent.time),
        droppedAttributesCount: timedEvent.droppedAttributesCount || 0,
    };
}
function createExportTraceServiceRequest(spans, encoder) {
    return {
        resourceSpans: spanRecordsToResourceSpans(spans, encoder),
    };
}
function createResourceMap(readableSpans) {
    const resourceMap = new Map();
    for (const record of readableSpans) {
        let ilsMap = resourceMap.get(record.resource);
        if (!ilsMap) {
            ilsMap = new Map();
            resourceMap.set(record.resource, ilsMap);
        }
        // TODO this is duplicated in basic tracer. Consolidate on a common helper in core
        const instrumentationScopeKey = `${record.instrumentationScope.name}@${record.instrumentationScope.version || ''}:${record.instrumentationScope.schemaUrl || ''}`;
        let records = ilsMap.get(instrumentationScopeKey);
        if (!records) {
            records = [];
            ilsMap.set(instrumentationScopeKey, records);
        }
        records.push(record);
    }
    return resourceMap;
}
function spanRecordsToResourceSpans(readableSpans, encoder) {
    const resourceMap = createResourceMap(readableSpans);
    const out = [];
    const entryIterator = resourceMap.entries();
    let entry = entryIterator.next();
    while (!entry.done) {
        const [resource, ilmMap] = entry.value;
        const scopeResourceSpans = [];
        const ilmIterator = ilmMap.values();
        let ilmEntry = ilmIterator.next();
        while (!ilmEntry.done) {
            const scopeSpans = ilmEntry.value;
            if (scopeSpans.length > 0) {
                const spans = scopeSpans.map(readableSpan => sdkSpanToOtlpSpan(readableSpan, encoder));
                scopeResourceSpans.push({
                    scope: createInstrumentationScope(scopeSpans[0].instrumentationScope, encoder),
                    spans: spans,
                    schemaUrl: scopeSpans[0].instrumentationScope.schemaUrl,
                });
            }
            ilmEntry = ilmIterator.next();
        }
        const processedResource = createResource(resource, encoder);
        const transformedSpans = {
            resource: processedResource,
            scopeSpans: scopeResourceSpans,
            schemaUrl: processedResource.schemaUrl,
        };
        out.push(transformedSpans);
        entry = entryIterator.next();
    }
    return out;
}
//# sourceMappingURL=internal.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+otlp-transformer@0.222.0+e40b0dfdd726a224/node_modules/@opentelemetry/otlp-transformer/build/esm/common/hex-to-binary.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
function intValue(charCode) {
    // 0-9
    if (charCode >= 48 && charCode <= 57) {
        return charCode - 48;
    }
    // a-f
    if (charCode >= 97 && charCode <= 102) {
        return charCode - 87;
    }
    // A-F
    return charCode - 55;
}
function hexToBinary(hexStr) {
    const buf = new Uint8Array(hexStr.length / 2);
    let offset = 0;
    for (let i = 0; i < hexStr.length; i += 2) {
        const hi = intValue(hexStr.charCodeAt(i));
        const lo = intValue(hexStr.charCodeAt(i + 1));
        buf[offset++] = (hi << 4) | lo;
    }
    return buf;
}
//# sourceMappingURL=hex-to-binary.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+otlp-transformer@0.222.0+e40b0dfdd726a224/node_modules/@opentelemetry/otlp-transformer/build/esm/common/utils.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */


function hrTimeToNanos(hrTime) {
    const NANOSECONDS = BigInt(1000000000);
    return (BigInt(Math.trunc(hrTime[0])) * NANOSECONDS + BigInt(Math.trunc(hrTime[1])));
}
function toLongBits(value) {
    const low = Number(BigInt.asUintN(32, value));
    const high = Number(BigInt.asUintN(32, value >> BigInt(32)));
    return { low, high };
}
function encodeAsLongBits(hrTime) {
    const nanos = hrTimeToNanos(hrTime);
    return toLongBits(nanos);
}
function encodeAsString(hrTime) {
    const nanos = hrTimeToNanos(hrTime);
    return nanos.toString();
}
const encodeTimestamp = typeof BigInt !== 'undefined' ? encodeAsString : hrTimeToNanoseconds;
function identity(value) {
    return value;
}
function optionalHexToBinary(str) {
    if (str === undefined)
        return undefined;
    return hexToBinary(str);
}
/**
 * Encoder for protobuf format.
 * Uses { high, low } timestamps and binary for span/trace IDs, leaves Uint8Array attributes as-is.
 */
const PROTOBUF_ENCODER = {
    encodeHrTime: encodeAsLongBits,
    encodeSpanContext: hexToBinary,
    encodeOptionalSpanContext: optionalHexToBinary,
    encodeUint8Array: identity,
};
/**
 * Encoder for JSON format.
 * Uses string timestamps, hex for span/trace IDs, and base64 for Uint8Array.
 */
const JSON_ENCODER = {
    encodeHrTime: encodeTimestamp,
    encodeSpanContext: identity,
    encodeOptionalSpanContext: identity,
    encodeUint8Array: (bytes) => {
        if (typeof Buffer !== 'undefined') {
            return Buffer.from(bytes).toString('base64');
        }
        // implementation note: not using spread operator and passing to
        // btoa to avoid stack overflow on large Uint8Arrays
        const chars = new Array(bytes.length);
        for (let i = 0; i < bytes.length; i++) {
            chars[i] = String.fromCharCode(bytes[i]);
        }
        return btoa(chars.join(''));
    },
};
//# sourceMappingURL=utils.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+otlp-transformer@0.222.0+e40b0dfdd726a224/node_modules/@opentelemetry/otlp-transformer/build/esm/trace/json/trace.js



const JsonTraceSerializer = {
    serializeRequest: (arg) => {
        const request = createExportTraceServiceRequest(arg, JSON_ENCODER);
        const encoder = new TextEncoder();
        return encoder.encode(JSON.stringify(request));
    },
    deserializeResponse: (arg) => {
        if (arg.length === 0) {
            return {};
        }
        const decoder = new TextDecoder();
        try {
            return JSON.parse(decoder.decode(arg));
        }
        catch (err) {
            diag_api_diag.warn(`Failed to parse trace export response: ${err.message}. Returning empty response`);
            return {};
        }
    },
};
//# sourceMappingURL=trace.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+otlp-transformer@0.222.0+e40b0dfdd726a224/node_modules/@opentelemetry/otlp-transformer/build/esm/trace/index.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
const TraceExporterMetricsHelper = {
    name: 'span',
    countItems: (request) => request.length,
};
//# sourceMappingURL=index.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+core@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/core/build/esm/ExportResult.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
var ExportResultCode;
(function (ExportResultCode) {
    ExportResultCode[ExportResultCode["SUCCESS"] = 0] = "SUCCESS";
    ExportResultCode[ExportResultCode["FAILED"] = 1] = "FAILED";
})(ExportResultCode || (ExportResultCode = {}));
//# sourceMappingURL=ExportResult.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+otlp-exporter-base@0.222.0+e40b0dfdd726a224/node_modules/@opentelemetry/otlp-exporter-base/build/esm/types.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
/**
 * Interface for handling error
 */
class OTLPExporterError extends Error {
    code;
    name = 'OTLPExporterError';
    data;
    constructor(message, code, data) {
        super(message);
        this.data = data;
        this.code = code;
    }
}
//# sourceMappingURL=types.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+otlp-exporter-base@0.222.0+e40b0dfdd726a224/node_modules/@opentelemetry/otlp-exporter-base/build/esm/logging-response-handler.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */

function isPartialSuccessResponse(response) {
    return Object.prototype.hasOwnProperty.call(response, 'partialSuccess');
}
/**
 * Default response handler that logs a partial success to the console.
 */
function createLoggingPartialSuccessResponseHandler() {
    return {
        handleResponse(response) {
            // Partial success MUST never be an empty object according the specification
            // see https://opentelemetry.io/docs/specs/otlp/#partial-success
            if (response == null ||
                !isPartialSuccessResponse(response) ||
                response.partialSuccess == null ||
                Object.keys(response.partialSuccess).length === 0) {
                return;
            }
            diag_api_diag.warn('Received Partial Success response:', JSON.stringify(response.partialSuccess));
        },
    };
}
//# sourceMappingURL=logging-response-handler.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+otlp-exporter-base@0.222.0+e40b0dfdd726a224/node_modules/@opentelemetry/otlp-exporter-base/build/esm/otlp-export-delegate.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */




class OTLPExportDelegate {
    _metrics;
    _diagLogger;
    _transport;
    _serializer;
    _responseHandler;
    _promiseQueue;
    _timeout;
    constructor(transport, serializer, responseHandler, promiseQueue, metrics, timeout) {
        this._transport = transport;
        this._serializer = serializer;
        this._responseHandler = responseHandler;
        this._promiseQueue = promiseQueue;
        this._timeout = timeout;
        this._diagLogger = diag_api_diag.createComponentLogger({
            namespace: 'OTLPExportDelegate',
        });
        this._metrics = metrics;
    }
    export(internalRepresentation, resultCallback) {
        this._diagLogger.debug('items to be sent', internalRepresentation);
        // don't do any work if too many exports are in progress.
        if (this._promiseQueue.hasReachedLimit()) {
            resultCallback({
                code: ExportResultCode.FAILED,
                error: new Error('Concurrent export limit reached'),
            });
            return;
        }
        const serializedRequest = this._serializer.serializeRequest(internalRepresentation);
        if (serializedRequest == null) {
            resultCallback({
                code: ExportResultCode.FAILED,
                error: new Error('Nothing to send'),
            });
            return;
        }
        const finishExport = this._metrics.startExport(internalRepresentation);
        this._promiseQueue.pushPromise(this._transport.send(serializedRequest, this._timeout).then(response => {
            if (response.status === 'success') {
                finishExport(undefined);
                if (response.data != null) {
                    try {
                        this._responseHandler.handleResponse(this._serializer.deserializeResponse(response.data));
                    }
                    catch (e) {
                        this._diagLogger.warn('Export succeeded but could not deserialize response - is the response specification compliant?', e, response.data);
                    }
                }
                // No matter the response, we can consider the export still successful.
                resultCallback({
                    code: ExportResultCode.SUCCESS,
                });
                return;
            }
            else if (response.status === 'failure' && response.error) {
                finishExport(response.error);
                resultCallback({
                    code: ExportResultCode.FAILED,
                    error: response.error,
                });
                return;
            }
            else if (response.status === 'retryable') {
                finishExport('export_max_retries');
                resultCallback({
                    code: ExportResultCode.FAILED,
                    error: response.error ??
                        new OTLPExporterError('Export failed with retryable status'),
                });
            }
            else {
                finishExport('export_failed');
                resultCallback({
                    code: ExportResultCode.FAILED,
                    error: new OTLPExporterError('Export failed with unknown error'),
                });
            }
        }, reason => {
            finishExport(reason);
            resultCallback({
                code: ExportResultCode.FAILED,
                error: reason,
            });
        }));
    }
    forceFlush() {
        return this._promiseQueue.awaitAll();
    }
    setMetrics(metrics) {
        this._metrics = metrics;
    }
    async shutdown() {
        this._diagLogger.debug('shutdown started');
        await this.forceFlush();
        this._transport.shutdown();
    }
}
/**
 * Creates a generic delegate for OTLP exports which only contains parts of the OTLP export that are shared across all
 * signals.
 */
function createOtlpExportDelegate(components, settings) {
    return new OTLPExportDelegate(components.transport, components.serializer, createLoggingPartialSuccessResponseHandler(), components.promiseHandler, components.metrics, settings.timeout);
}
//# sourceMappingURL=otlp-export-delegate.js.map
// EXTERNAL MODULE: external "zlib"
var external_zlib_ = __webpack_require__(74075);
// EXTERNAL MODULE: external "stream"
var external_stream_ = __webpack_require__(27910);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+otlp-exporter-base@0.222.0+e40b0dfdd726a224/node_modules/@opentelemetry/otlp-exporter-base/build/esm/is-export-retryable.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
function isExportHTTPErrorRetryable(statusCode) {
    return (statusCode === 429 ||
        statusCode === 502 ||
        statusCode === 503 ||
        statusCode === 504);
}
function parseRetryAfterToMills(retryAfter) {
    if (retryAfter == null) {
        return undefined;
    }
    const seconds = Number.parseInt(retryAfter, 10);
    if (Number.isInteger(seconds)) {
        return seconds > 0 ? seconds * 1000 : -1;
    }
    // https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Retry-After#directives
    const delay = new Date(retryAfter).getTime() - Date.now();
    if (delay >= 0) {
        return delay;
    }
    return 0;
}
//# sourceMappingURL=is-export-retryable.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+otlp-exporter-base@0.222.0+e40b0dfdd726a224/node_modules/@opentelemetry/otlp-exporter-base/build/esm/version.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
// this is autogenerated file, see scripts/version-update.js
const esm_version_VERSION = '0.222.0';
//# sourceMappingURL=version.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+otlp-exporter-base@0.222.0+e40b0dfdd726a224/node_modules/@opentelemetry/otlp-exporter-base/build/esm/transport/http-transport-utils.js





const DEFAULT_USER_AGENT = `OTel-OTLP-Exporter-JavaScript/${esm_version_VERSION}`;
/**
 * Maximum response body size (4 MB) that the HTTP transport will read.
 * If the server sends more data the connection is destroyed and the export
 * is treated as a non-retryable error regardless of status code.
 */
const MAX_RESPONSE_BODY_SIZE = 4 * 1024 * 1024;
/**
 * Sends data using http
 * @param request
 * @param url
 * @param headers
 * @param compression
 * @param userAgent
 * @param agent
 * @param data
 * @param timeoutMillis
 */
function sendWithHttp(request, url, headers, compression, userAgent, agent, data, timeoutMillis) {
    return new Promise(resolve => {
        const parsedUrl = new URL(url);
        if (userAgent) {
            headers['User-Agent'] = `${userAgent} ${DEFAULT_USER_AGENT}`;
        }
        else {
            headers['User-Agent'] = DEFAULT_USER_AGENT;
        }
        const options = {
            method: 'POST',
            headers,
            agent,
        };
        const req = request(parsedUrl, options, (res) => {
            const responseData = [];
            let responseSize = 0;
            res.on('data', (chunk) => {
                responseSize += chunk.length;
                if (responseSize > MAX_RESPONSE_BODY_SIZE) {
                    const sizeError = new Error(`OTLP export response body exceeded size limit of ${MAX_RESPONSE_BODY_SIZE} bytes`);
                    // Oversized responses are treated as non-retryable errors
                    // regardless of status code.
                    // Resolve before destroying: res.destroy() tears down the socket which
                    // triggers ECONNRESET on req.on('error'), so, resolving first makes that
                    // a no-op. res.on('error') does not fire because destroy() is called
                    // without an error argument.
                    resolve({ status: 'failure', error: sizeError });
                    res.destroy();
                    return;
                }
                responseData.push(chunk);
            });
            res.on('end', () => {
                if (res.statusCode && res.statusCode <= 299) {
                    resolve({
                        status: 'success',
                        data: Buffer.concat(responseData),
                    });
                }
                else if (res.statusCode &&
                    isExportHTTPErrorRetryable(res.statusCode)) {
                    resolve({
                        status: 'retryable',
                        retryInMillis: parseRetryAfterToMills(res.headers['retry-after']),
                    });
                }
                else {
                    const error = new OTLPExporterError(res.statusMessage, res.statusCode, Buffer.concat(responseData).toString());
                    resolve({
                        status: 'failure',
                        error,
                    });
                }
            });
            res.on('error', (error) => {
                // Note: 'end' may still be emitted after 'error' on the same response object, since we're resolving a promise,
                // the first call to resolve() will determine the final state.
                if (res.statusCode && res.statusCode <= 299) {
                    // If the response is successful but an error occurs while reading the response,
                    // we consider it a success since the data has been sent successfully.
                    resolve({
                        status: 'success',
                    });
                }
                else if (res.statusCode &&
                    isExportHTTPErrorRetryable(res.statusCode)) {
                    resolve({
                        status: 'retryable',
                        error: error,
                        retryInMillis: parseRetryAfterToMills(res.headers['retry-after']),
                    });
                }
                else {
                    resolve({
                        status: 'failure',
                        error,
                    });
                }
            });
        });
        req.setTimeout(timeoutMillis, () => {
            req.destroy();
            resolve({
                status: 'retryable',
                error: new Error('Request timed out'),
            });
        });
        req.on('error', (error) => {
            if (isHttpTransportNetworkErrorRetryable(error)) {
                resolve({
                    status: 'retryable',
                    error,
                });
            }
            else {
                resolve({
                    status: 'failure',
                    error,
                });
            }
        });
        compressAndSend(req, compression, data, (error) => {
            resolve({
                status: 'failure',
                error,
            });
        });
    });
}
function compressAndSend(req, compression, data, onError) {
    let dataStream = readableFromUint8Array(data);
    if (compression === 'gzip') {
        req.setHeader('Content-Encoding', 'gzip');
        dataStream = dataStream
            .on('error', onError)
            .pipe(external_zlib_.createGzip())
            .on('error', onError);
    }
    dataStream.pipe(req).on('error', onError);
}
function readableFromUint8Array(buff) {
    const readable = new external_stream_.Readable();
    readable.push(buff);
    readable.push(null);
    return readable;
}
function isHttpTransportNetworkErrorRetryable(error) {
    const RETRYABLE_NETWORK_ERROR_CODES = new Set([
        'ECONNRESET',
        'ECONNREFUSED',
        'EPIPE',
        'ETIMEDOUT',
        'EAI_AGAIN',
        'ENOTFOUND',
        'ENETUNREACH',
        'EHOSTUNREACH',
    ]);
    if ('code' in error && typeof error.code === 'string') {
        return RETRYABLE_NETWORK_ERROR_CODES.has(error.code);
    }
    return false;
}
//# sourceMappingURL=http-transport-utils.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+otlp-exporter-base@0.222.0+e40b0dfdd726a224/node_modules/@opentelemetry/otlp-exporter-base/build/esm/transport/http-exporter-transport.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */

class HttpExporterTransport {
    _utils = null;
    _parameters;
    constructor(parameters) {
        this._parameters = parameters;
    }
    async send(data, timeoutMillis) {
        const { agent, request } = await this._loadUtils();
        const headers = await this._parameters.headers();
        return sendWithHttp(request, this._parameters.url, headers, this._parameters.compression, this._parameters.userAgent, agent, data, timeoutMillis);
    }
    shutdown() {
        // intentionally left empty, nothing to do.
    }
    async _loadUtils() {
        let utils = this._utils;
        if (utils === null) {
            const protocol = new URL(this._parameters.url).protocol;
            const [agent, request] = await Promise.all([
                this._parameters.agentFactory(protocol),
                requestFunctionFactory(protocol),
            ]);
            utils = this._utils = { agent, request };
        }
        return utils;
    }
}
async function requestFunctionFactory(protocol) {
    const module = protocol === 'http:' ? Promise.resolve(/* import() */).then(__webpack_require__.t.bind(__webpack_require__, 81630, 23)) : Promise.resolve(/* import() */).then(__webpack_require__.t.bind(__webpack_require__, 55591, 23));
    const { request } = await module;
    return request;
}
function createHttpExporterTransport(parameters) {
    return new HttpExporterTransport(parameters);
}
//# sourceMappingURL=http-exporter-transport.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+otlp-exporter-base@0.222.0+e40b0dfdd726a224/node_modules/@opentelemetry/otlp-exporter-base/build/esm/bounded-queue-export-promise-handler.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
class BoundedQueueExportPromiseHandler {
    _concurrencyLimit;
    _sendingPromises = [];
    /**
     * @param concurrencyLimit maximum promises allowed in a queue at the same time.
     */
    constructor(concurrencyLimit) {
        this._concurrencyLimit = concurrencyLimit;
    }
    pushPromise(promise) {
        if (this.hasReachedLimit()) {
            throw new Error('Concurrency Limit reached');
        }
        this._sendingPromises.push(promise);
        const popPromise = () => {
            const index = this._sendingPromises.indexOf(promise);
            void this._sendingPromises.splice(index, 1);
        };
        promise.then(popPromise, popPromise);
    }
    hasReachedLimit() {
        return this._sendingPromises.length >= this._concurrencyLimit;
    }
    async awaitAll() {
        await Promise.all(this._sendingPromises);
    }
}
/**
 * Promise queue for keeping track of export promises. Finished promises will be auto-dequeued.
 * Allows for awaiting all promises in the queue.
 */
function createBoundedQueueExportPromiseHandler(options) {
    return new BoundedQueueExportPromiseHandler(options.concurrencyLimit);
}
//# sourceMappingURL=bounded-queue-export-promise-handler.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+otlp-exporter-base@0.222.0+e40b0dfdd726a224/node_modules/@opentelemetry/otlp-exporter-base/build/esm/retrying-transport.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */

const MAX_ATTEMPTS = 5;
const INITIAL_BACKOFF = 1000;
const MAX_BACKOFF = 5000;
const BACKOFF_MULTIPLIER = 1.5;
const JITTER = 0.2;
/**
 * Get a pseudo-random jitter that falls in the range of [-JITTER, +JITTER]
 */
function getJitter() {
    return Math.random() * (2 * JITTER) - JITTER;
}
class RetryingTransport {
    _transport;
    constructor(transport) {
        this._transport = transport;
    }
    retry(data, timeoutMillis, inMillis) {
        return new Promise((resolve, reject) => {
            setTimeout(() => {
                this._transport.send(data, timeoutMillis).then(resolve, reject);
            }, inMillis);
        });
    }
    async send(data, timeoutMillis) {
        let attempts = MAX_ATTEMPTS;
        let nextBackoff = INITIAL_BACKOFF;
        const deadline = Date.now() + timeoutMillis;
        let result = await this._transport.send(data, timeoutMillis);
        while (result.status === 'retryable' && attempts > 0) {
            attempts--;
            // use maximum of computed backoff and 0 to avoid negative timeouts
            const backoff = Math.max(Math.min(nextBackoff * (1 + getJitter()), MAX_BACKOFF), 0);
            nextBackoff = nextBackoff * BACKOFF_MULTIPLIER;
            const retryInMillis = result.retryInMillis ?? backoff;
            // return when expected retry time is after the export deadline.
            const remainingTimeoutMillis = deadline - Date.now();
            if (retryInMillis > remainingTimeoutMillis) {
                diag_api_diag.info(`Export retry time ${Math.round(retryInMillis)}ms exceeds remaining timeout ${Math.round(remainingTimeoutMillis)}ms, not retrying further.`);
                return result;
            }
            diag_api_diag.verbose(`Scheduling export retry in ${Math.round(retryInMillis)}ms`);
            result = await this.retry(data, remainingTimeoutMillis, retryInMillis);
        }
        if (result.status === 'success') {
            diag_api_diag.verbose(`Export succeeded after ${MAX_ATTEMPTS - attempts} retry attempts.`);
        }
        else if (result.status === 'retryable') {
            diag_api_diag.info(`Export failed after maximum retry attempts (${MAX_ATTEMPTS}).`);
        }
        else {
            diag_api_diag.info(`Export failed with non-retryable error: ${result.error}`);
        }
        return result;
    }
    shutdown() {
        return this._transport.shutdown();
    }
}
/**
 * Creates an Exporter Transport that retries on 'retryable' response.
 */
function createRetryingTransport(options) {
    return new RetryingTransport(options.transport);
}
//# sourceMappingURL=retrying-transport.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+otlp-exporter-base@0.222.0+e40b0dfdd726a224/node_modules/@opentelemetry/otlp-exporter-base/build/esm/semconv.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
/*
 * This file contains a copy of unstable semantic convention definitions
 * used by this package.
 * @see https://github.com/open-telemetry/opentelemetry-js/tree/main/semantic-conventions#unstable-semconv
 */
/**
 * [HTTP response status code](https://tools.ietf.org/html/rfc7231#section-6).
 *
 * @example 200
 */
const semconv_ATTR_HTTP_RESPONSE_STATUS_CODE = 'http.response.status_code';
/**
 * A name uniquely identifying the instance of the OpenTelemetry component within its containing SDK instance.
 *
 * @example otlp_grpc_span_exporter/0
 * @example custom-name
 *
 * @note Implementations **SHOULD** ensure a low cardinality for this attribute, even across application or SDK restarts.
 * E.g. implementations **MUST NOT** use UUIDs as values for this attribute.
 *
 * Implementations **MAY** achieve these goals by following a `<otel.component.type>/<instance-counter>` pattern, e.g. `batching_span_processor/0`.
 * Hereby `otel.component.type` refers to the corresponding attribute value of the component.
 *
 * The value of `instance-counter` **MAY** be automatically assigned by the component and uniqueness within the enclosing SDK instance **MUST** be guaranteed.
 * For example, `<instance-counter>` **MAY** be implemented by using a monotonically increasing counter (starting with `0`), which is incremented every time an
 * instance of the given component type is started.
 *
 * With this implementation, for example the first Batching Span Processor would have `batching_span_processor/0`
 * as `otel.component.name`, the second one `batching_span_processor/1` and so on.
 * These values will therefore be reused in the case of an application restart.
 *
 * @experimental This attribute is experimental and is subject to breaking changes in minor releases of `@opentelemetry/semantic-conventions`.
 */
const ATTR_OTEL_COMPONENT_NAME = 'otel.component.name';
/**
 * A name identifying the type of the OpenTelemetry component.
 *
 * @example batching_span_processor
 * @example com.example.MySpanExporter
 *
 * @note If none of the standardized values apply, implementations **SHOULD** use the language-defined name of the type.
 * E.g. for Java the fully qualified classname **SHOULD** be used in this case.
 *
 * @experimental This attribute is experimental and is subject to breaking changes in minor releases of `@opentelemetry/semantic-conventions`.
 */
const ATTR_OTEL_COMPONENT_TYPE = 'otel.component.type';
/**
 * Server domain name if available without reverse DNS lookup; otherwise, IP address or Unix domain socket name.
 *
 * @example example.com
 * @example 10.1.2.80
 * @example /tmp/my.sock
 *
 * @note When observed from the client side, and when communicating through an intermediary, `server.address` **SHOULD** represent the server address behind any intermediaries, for example proxies, if it's available.
 */
const semconv_ATTR_SERVER_ADDRESS = 'server.address';
/**
 * Server port number.
 *
 * @example 80
 * @example 8080
 * @example 443
 *
 * @note When observed from the client side, and when communicating through an intermediary, `server.port` **SHOULD** represent the server port behind any intermediaries, for example proxies, if it's available.
 */
const semconv_ATTR_SERVER_PORT = 'server.port';
/**
 * Describes a class of error the operation ended with.
 *
 * @example timeout
 * @example java.net.UnknownHostException
 * @example server_certificate_invalid
 * @example 500
 *
 * @note The `error.type` **SHOULD** be predictable, and **SHOULD** have low cardinality.
 *
 * When `error.type` is set to a type (e.g., an exception type), its
 * canonical class name identifying the type within the artifact **SHOULD** be used.
 *
 * Instrumentations **SHOULD** document the list of errors they report.
 *
 * The cardinality of `error.type` within one instrumentation library **SHOULD** be low.
 * Telemetry consumers that aggregate data from multiple instrumentation libraries and applications
 * should be prepared for `error.type` to have high cardinality at query time when no
 * additional filters are applied.
 *
 * If the operation has completed successfully, instrumentations **SHOULD NOT** set `error.type`.
 *
 * If a specific domain defines its own set of error identifiers (such as HTTP or RPC status codes),
 * it's **RECOMMENDED** to:
 *
 *   - Use a domain-specific attribute
 *   - Set `error.type` to capture all errors, regardless of whether they are defined within the domain-specific set or not.
 */
const semconv_ATTR_ERROR_TYPE = 'error.type';
//# sourceMappingURL=semconv.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+otlp-exporter-base@0.222.0+e40b0dfdd726a224/node_modules/@opentelemetry/otlp-exporter-base/build/esm/ExporterMetrics.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */




const componentCounter = new Map();
/**
 * Generates `otel.sdk.exporter.*` metrics.
 * https://opentelemetry.io/docs/specs/semconv/otel/sdk-metrics
 */
class ExporterMetrics {
    inflight;
    exported;
    duration;
    standardAttrs;
    responseAttributesFromError;
    helper;
    constructor(options) {
        const { componentType, metricsHelper, meterProvider, url, responseAttributesFromError, } = options;
        this.responseAttributesFromError = responseAttributesFromError;
        const meter = meterProvider
            ? meterProvider.getMeter('@opentelemetry/otlp-exporter', esm_version_VERSION)
            : createNoopMeter();
        const counter = componentCounter.get(componentType) ?? 0;
        componentCounter.set(componentType, counter + 1);
        this.standardAttrs = {
            [ATTR_OTEL_COMPONENT_TYPE]: componentType,
            [ATTR_OTEL_COMPONENT_NAME]: `${componentType}/${counter}`,
        };
        if (url) {
            // URLs may exclude scheme for gRPC endpoints, but in this case they always
            // have a port number. Because the URL constructor requires a scheme, we
            // can still handle gRPC endpoints by prepending an arbitrary scheme.
            let urlToParse = url;
            if (!url.includes('://')) {
                urlToParse = `http://${url}`;
            }
            try {
                const parsedUrl = new URL(urlToParse);
                this.standardAttrs[semconv_ATTR_SERVER_ADDRESS] = parsedUrl.hostname;
                let port = undefined;
                if (parsedUrl.port) {
                    port = Number(parsedUrl.port);
                }
                else if (parsedUrl.protocol === 'http:') {
                    port = 80;
                }
                else if (parsedUrl.protocol === 'https:') {
                    port = 443;
                }
                if (typeof port === 'number') {
                    this.standardAttrs[semconv_ATTR_SERVER_PORT] = port;
                }
            }
            catch {
                // In practice, URLs will be valid or something else will break. Better to let that
                // inform the user than an exception in this internal code and proceed best-effort
                // here.
            }
        }
        this.helper = metricsHelper;
        this.inflight = meter.createUpDownCounter(`otel.sdk.exporter.${this.helper.name}.inflight`, {
            unit: `{${this.helper.name}}`,
            description: `The number of ${this.helper.name}s which were passed to the exporter, but that have not been exported yet (neither successful, nor failed).`,
        });
        this.exported = meter.createCounter(`otel.sdk.exporter.${this.helper.name}.exported`, {
            unit: `{${this.helper.name}}`,
            description: `The number of ${this.helper.name}s for which the export has finished, either successful or failed.`,
        });
        this.duration = meter.createHistogram('otel.sdk.exporter.operation.duration', {
            unit: 's',
            description: 'The duration of exporting a batch of telemetry records.',
            advice: {
                explicitBucketBoundaries: [],
            },
        });
    }
    startExport(request) {
        const numItems = this.helper.countItems(request);
        const startTime = hrTime();
        this.inflight.add(numItems, this.standardAttrs);
        return (error) => {
            const endTime = hrTime();
            this.inflight.add(-numItems, this.standardAttrs);
            const exportedAttrs = error
                ? {
                    ...this.standardAttrs,
                    [semconv_ATTR_ERROR_TYPE]: error instanceof Error ? error.name : 'export_failed',
                }
                : this.standardAttrs;
            this.exported.add(numItems, exportedAttrs);
            const durationAttrs = {
                ...exportedAttrs,
                ...this.responseAttributesFromError(error),
            };
            const duration = hrTimeToMilliseconds(hrTimeDuration(startTime, endTime)) / 1000;
            this.duration.record(duration, durationAttrs);
        };
    }
}
//# sourceMappingURL=ExporterMetrics.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+otlp-exporter-base@0.222.0+e40b0dfdd726a224/node_modules/@opentelemetry/otlp-exporter-base/build/esm/otlp-http-export-delegate.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */







function createOtlpHttpExporterMetrics(metricsComponentType, exporterMetricsHelper, url, meterProvider) {
    return new ExporterMetrics({
        componentType: metricsComponentType,
        metricsHelper: exporterMetricsHelper,
        url,
        meterProvider,
        responseAttributesFromError: (error) => {
            if (!error) {
                return {
                    [semconv_ATTR_HTTP_RESPONSE_STATUS_CODE]: 200,
                };
            }
            if (!(error instanceof OTLPExporterError)) {
                return {};
            }
            return {
                [semconv_ATTR_HTTP_RESPONSE_STATUS_CODE]: error.code,
            };
        },
    });
}
function createOtlpHttpExportDelegate(options, serializer, metricsComponentType, exporterMetricsHelper, meterProvider) {
    return createOtlpExportDelegate({
        transport: createRetryingTransport({
            transport: createHttpExporterTransport(options),
        }),
        serializer: serializer,
        promiseHandler: createBoundedQueueExportPromiseHandler(options),
        metrics: createOtlpHttpExporterMetrics(metricsComponentType, exporterMetricsHelper, options.url, meterProvider),
    }, { timeout: options.timeoutMillis });
}
//# sourceMappingURL=otlp-http-export-delegate.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+otlp-exporter-base@0.222.0+e40b0dfdd726a224/node_modules/@opentelemetry/otlp-exporter-base/build/esm/configuration/shared-configuration.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
function validateTimeoutMillis(timeoutMillis) {
    if (Number.isFinite(timeoutMillis) && timeoutMillis > 0) {
        return timeoutMillis;
    }
    throw new Error(`Configuration: timeoutMillis is invalid, expected number greater than 0 (actual: '${timeoutMillis}')`);
}
function wrapStaticHeadersInFunction(headers) {
    if (headers == null) {
        return undefined;
    }
    return async () => headers;
}
/**
 * @param userProvidedConfiguration  Configuration options provided by the user in code.
 * @param fallbackConfiguration Fallback to use when the {@link userProvidedConfiguration} does not specify an option.
 * @param defaultConfiguration The defaults as defined by the exporter specification
 */
function mergeOtlpSharedConfigurationWithDefaults(userProvidedConfiguration, fallbackConfiguration, defaultConfiguration) {
    return {
        timeoutMillis: validateTimeoutMillis(userProvidedConfiguration.timeoutMillis ??
            fallbackConfiguration.timeoutMillis ??
            defaultConfiguration.timeoutMillis),
        concurrencyLimit: userProvidedConfiguration.concurrencyLimit ??
            fallbackConfiguration.concurrencyLimit ??
            defaultConfiguration.concurrencyLimit,
        compression: userProvidedConfiguration.compression ??
            fallbackConfiguration.compression ??
            defaultConfiguration.compression,
    };
}
function getSharedConfigurationDefaults() {
    return {
        timeoutMillis: 10000,
        concurrencyLimit: 30,
        compression: 'none',
    };
}
//# sourceMappingURL=shared-configuration.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+otlp-exporter-base@0.222.0+e40b0dfdd726a224/node_modules/@opentelemetry/otlp-exporter-base/build/esm/util.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */

/**
 * Parses headers from config leaving only those that have defined values
 * @param partialHeaders
 */
function validateAndNormalizeHeaders(partialHeaders) {
    const headers = {};
    Object.entries(partialHeaders ?? {}).forEach(([key, value]) => {
        if (typeof value !== 'undefined') {
            headers[key] = String(value);
        }
        else {
            diag_api_diag.warn(`Header "${key}" has invalid value (${value}) and will be ignored`);
        }
    });
    return headers;
}
//# sourceMappingURL=util.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+otlp-exporter-base@0.222.0+e40b0dfdd726a224/node_modules/@opentelemetry/otlp-exporter-base/build/esm/configuration/otlp-http-configuration.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */


function mergeHeaders(userProvidedHeaders, fallbackHeaders, defaultHeaders) {
    return async () => {
        const requiredHeaders = {
            ...(await defaultHeaders()),
        };
        const headers = {};
        // add fallback ones first
        if (fallbackHeaders != null) {
            Object.assign(headers, await fallbackHeaders());
        }
        // override with user-provided ones
        if (userProvidedHeaders != null) {
            Object.assign(headers, validateAndNormalizeHeaders(await userProvidedHeaders()));
        }
        // override required ones.
        return Object.assign(headers, requiredHeaders);
    };
}
function validateUserProvidedUrl(url) {
    if (url == null) {
        return undefined;
    }
    try {
        // NOTE: In non-browser environments, `globalThis.location` will be `undefined`.
        const base = globalThis.location?.href;
        return new URL(url, base).href;
    }
    catch {
        throw new Error(`Configuration: Could not parse user-provided export URL: '${url}'`);
    }
}
/**
 * @param userProvidedConfiguration  Configuration options provided by the user in code.
 * @param fallbackConfiguration Fallback to use when the {@link userProvidedConfiguration} does not specify an option.
 * @param defaultConfiguration The defaults as defined by the exporter specification
 */
function mergeOtlpHttpConfigurationWithDefaults(userProvidedConfiguration, fallbackConfiguration, defaultConfiguration) {
    return {
        ...mergeOtlpSharedConfigurationWithDefaults(userProvidedConfiguration, fallbackConfiguration, defaultConfiguration),
        headers: mergeHeaders(userProvidedConfiguration.headers, fallbackConfiguration.headers, defaultConfiguration.headers),
        url: validateUserProvidedUrl(userProvidedConfiguration.url) ??
            fallbackConfiguration.url ??
            defaultConfiguration.url,
    };
}
function getHttpConfigurationDefaults(requiredHeaders, signalResourcePath) {
    return {
        ...getSharedConfigurationDefaults(),
        headers: async () => requiredHeaders,
        url: 'http://localhost:4318/' + signalResourcePath,
    };
}
//# sourceMappingURL=otlp-http-configuration.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+otlp-exporter-base@0.222.0+e40b0dfdd726a224/node_modules/@opentelemetry/otlp-exporter-base/build/esm/configuration/otlp-node-http-configuration.js

function httpAgentFactoryFromOptions(options) {
    return async (protocol) => {
        const isInsecure = protocol === 'http:';
        const module = isInsecure ? Promise.resolve(/* import() */).then(__webpack_require__.t.bind(__webpack_require__, 81630, 23)) : Promise.resolve(/* import() */).then(__webpack_require__.t.bind(__webpack_require__, 55591, 23));
        const { Agent } = await module;
        if (isInsecure) {
            // eslint-disable-next-line @typescript-eslint/no-unused-vars -- these props should not be used in agent options
            const { ca, cert, key, ...insecureOptions } = options;
            return new Agent(insecureOptions);
        }
        return new Agent(options);
    };
}
/**
 * @param userProvidedConfiguration  Configuration options provided by the user in code.
 * @param fallbackConfiguration Fallback to use when the {@link userProvidedConfiguration} does not specify an option.
 * @param defaultConfiguration The defaults as defined by the exporter specification
 */
function mergeOtlpNodeHttpConfigurationWithDefaults(userProvidedConfiguration, fallbackConfiguration, defaultConfiguration) {
    return {
        ...mergeOtlpHttpConfigurationWithDefaults(userProvidedConfiguration, fallbackConfiguration, defaultConfiguration),
        agentFactory: userProvidedConfiguration.agentFactory ??
            fallbackConfiguration.agentFactory ??
            defaultConfiguration.agentFactory,
        userAgent: userProvidedConfiguration.userAgent,
    };
}
function getNodeHttpConfigurationDefaults(requiredHeaders, signalResourcePath) {
    return {
        ...getHttpConfigurationDefaults(requiredHeaders, signalResourcePath),
        agentFactory: httpAgentFactoryFromOptions({ keepAlive: true }),
    };
}
//# sourceMappingURL=otlp-node-http-configuration.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+core@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/core/build/esm/baggage/constants.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
const BAGGAGE_KEY_PAIR_SEPARATOR = '=';
const BAGGAGE_PROPERTIES_SEPARATOR = ';';
const BAGGAGE_ITEMS_SEPARATOR = ',';
// Name of the http header used to propagate the baggage
const BAGGAGE_HEADER = 'baggage';
// Maximum number of name-value pairs allowed by w3c spec
const BAGGAGE_MAX_NAME_VALUE_PAIRS = 180;
// Maximum number of bytes per a single name-value pair allowed by w3c spec
const BAGGAGE_MAX_PER_NAME_VALUE_PAIRS = 4096;
// Maximum total length of all name-value pairs allowed by w3c spec
const BAGGAGE_MAX_TOTAL_LENGTH = 8192;
//# sourceMappingURL=constants.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+core@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/core/build/esm/baggage/utils.js


function serializeKeyPairs(keyPairs) {
    return keyPairs.reduce((hValue, current) => {
        const value = `${hValue}${hValue !== '' ? BAGGAGE_ITEMS_SEPARATOR : ''}${current}`;
        return value.length > BAGGAGE_MAX_TOTAL_LENGTH ? hValue : value;
    }, '');
}
function getKeyPairs(baggage) {
    return baggage.getAllEntries().map(([key, value]) => {
        let entry = `${encodeURIComponent(key)}=${encodeURIComponent(value.value)}`;
        // include opaque metadata if provided
        // NOTE: we intentionally don't URI-encode the metadata - that responsibility falls on the metadata implementation
        if (value.metadata !== undefined) {
            entry += BAGGAGE_PROPERTIES_SEPARATOR + value.metadata.toString();
        }
        return entry;
    });
}
function parsePairKeyValue(entry) {
    if (!entry)
        return;
    const metadataSeparatorIndex = entry.indexOf(BAGGAGE_PROPERTIES_SEPARATOR);
    const keyPairPart = metadataSeparatorIndex === -1
        ? entry
        : entry.substring(0, metadataSeparatorIndex);
    const separatorIndex = keyPairPart.indexOf(BAGGAGE_KEY_PAIR_SEPARATOR);
    if (separatorIndex <= 0)
        return;
    const rawKey = keyPairPart.substring(0, separatorIndex).trim();
    const rawValue = keyPairPart.substring(separatorIndex + 1).trim();
    if (!rawKey || !rawValue)
        return;
    let key;
    let value;
    try {
        key = decodeURIComponent(rawKey);
        value = decodeURIComponent(rawValue);
    }
    catch {
        return;
    }
    let metadata;
    if (metadataSeparatorIndex !== -1 &&
        metadataSeparatorIndex < entry.length - 1) {
        const metadataString = entry.substring(metadataSeparatorIndex + 1);
        metadata = baggageEntryMetadataFromString(metadataString);
    }
    return { key, value, metadata };
}
/**
 * Parses a single baggage header string into the provided record, applying limits defined in this package.
 * Uses indexOf/substring in a while loop to avoid allocating a full array of split entries.
 * Returns the updated pair count so callers can track totals across multiple header values.
 */
function parseBaggageHeaderString(value, baggage, count, totalSize) {
    let start = 0;
    while (start < value.length && count < BAGGAGE_MAX_NAME_VALUE_PAIRS) {
        const end = value.indexOf(BAGGAGE_ITEMS_SEPARATOR, start);
        const entryEnd = end === -1 ? value.length : end;
        const entryLength = entryEnd - start;
        if (entryLength <= BAGGAGE_MAX_PER_NAME_VALUE_PAIRS) {
            const keyPair = parsePairKeyValue(value.substring(start, entryEnd));
            if (keyPair) {
                // Comma separator is counted for every accepted entry after the first
                const entrySize = (count === 0 ? 0 : 1) + entryLength;
                if (totalSize + entrySize > BAGGAGE_MAX_TOTAL_LENGTH)
                    break;
                baggage[keyPair.key] = keyPair.metadata
                    ? { value: keyPair.value, metadata: keyPair.metadata }
                    : { value: keyPair.value };
                count++;
                totalSize += entrySize;
            }
        }
        if (end === -1)
            break;
        start = end + 1;
    }
    return [count, totalSize];
}
/**
 * Parse a string serialized in the baggage HTTP Format (without metadata):
 * https://github.com/w3c/baggage/blob/master/baggage/HTTP_HEADER_FORMAT.md
 */
function parseKeyPairsIntoRecord(value) {
    const result = {};
    if (typeof value === 'string' && value.length > 0) {
        value.split(BAGGAGE_ITEMS_SEPARATOR).forEach(entry => {
            const keyPair = parsePairKeyValue(entry);
            if (keyPair !== undefined && keyPair.value.length > 0) {
                result[keyPair.key] = keyPair.value;
            }
        });
    }
    return result;
}
//# sourceMappingURL=utils.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+otlp-exporter-base@0.222.0+e40b0dfdd726a224/node_modules/@opentelemetry/otlp-exporter-base/build/esm/configuration/shared-env-configuration.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */


function parseAndValidateTimeoutFromEnv(timeoutEnvVar) {
    const envTimeout = getNumberFromEnv(timeoutEnvVar);
    if (envTimeout != null) {
        if (Number.isFinite(envTimeout) && envTimeout > 0) {
            return envTimeout;
        }
        diag_api_diag.warn(`Configuration: ${timeoutEnvVar} is invalid, expected number greater than 0 (actual: ${envTimeout})`);
    }
    return undefined;
}
function getTimeoutFromEnv(signalIdentifier) {
    const specificTimeout = parseAndValidateTimeoutFromEnv(`OTEL_EXPORTER_OTLP_${signalIdentifier}_TIMEOUT`);
    const nonSpecificTimeout = parseAndValidateTimeoutFromEnv('OTEL_EXPORTER_OTLP_TIMEOUT');
    return specificTimeout ?? nonSpecificTimeout;
}
function parseAndValidateCompressionFromEnv(compressionEnvVar) {
    const compression = getStringFromEnv(compressionEnvVar)?.trim();
    if (compression == null || compression === 'none' || compression === 'gzip') {
        return compression;
    }
    diag_api_diag.warn(`Configuration: ${compressionEnvVar} is invalid, expected 'none' or 'gzip' (actual: '${compression}')`);
    return undefined;
}
function getCompressionFromEnv(signalIdentifier) {
    const specificCompression = parseAndValidateCompressionFromEnv(`OTEL_EXPORTER_OTLP_${signalIdentifier}_COMPRESSION`);
    const nonSpecificCompression = parseAndValidateCompressionFromEnv('OTEL_EXPORTER_OTLP_COMPRESSION');
    return specificCompression ?? nonSpecificCompression;
}
function getSharedConfigurationFromEnvironment(signalIdentifier) {
    return {
        timeoutMillis: getTimeoutFromEnv(signalIdentifier),
        compression: getCompressionFromEnv(signalIdentifier),
    };
}
//# sourceMappingURL=shared-env-configuration.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+otlp-exporter-base@0.222.0+e40b0dfdd726a224/node_modules/@opentelemetry/otlp-exporter-base/build/esm/configuration/otlp-node-http-env-configuration.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */







function getStaticHeadersFromEnv(signalIdentifier) {
    const signalSpecificRawHeaders = getStringFromEnv(`OTEL_EXPORTER_OTLP_${signalIdentifier}_HEADERS`);
    const nonSignalSpecificRawHeaders = getStringFromEnv('OTEL_EXPORTER_OTLP_HEADERS');
    const signalSpecificHeaders = parseKeyPairsIntoRecord(signalSpecificRawHeaders);
    const nonSignalSpecificHeaders = parseKeyPairsIntoRecord(nonSignalSpecificRawHeaders);
    if (Object.keys(signalSpecificHeaders).length === 0 &&
        Object.keys(nonSignalSpecificHeaders).length === 0) {
        return undefined;
    }
    // headers are combined instead of overwritten, with the specific headers taking precedence over
    // the non-specific ones.
    return Object.assign({}, parseKeyPairsIntoRecord(nonSignalSpecificRawHeaders), parseKeyPairsIntoRecord(signalSpecificRawHeaders));
}
function appendRootPathToUrlIfNeeded(url) {
    try {
        const parsedUrl = new URL(url);
        // This will automatically append '/' if there's no root path.
        return parsedUrl.toString();
    }
    catch {
        diag_api_diag.warn(`Configuration: Could not parse environment-provided export URL: '${url}', falling back to undefined`);
        return undefined;
    }
}
function appendResourcePathToUrl(url, path) {
    try {
        // just try to parse, if it fails we catch and warn.
        new URL(url);
    }
    catch {
        diag_api_diag.warn(`Configuration: Could not parse environment-provided export URL: '${url}', falling back to undefined`);
        return undefined;
    }
    if (!url.endsWith('/')) {
        url = url + '/';
    }
    url += path;
    try {
        // just try to parse, if it fails we catch and warn.
        new URL(url);
    }
    catch {
        diag_api_diag.warn(`Configuration: Provided URL appended with '${path}' is not a valid URL, using 'undefined' instead of '${url}'`);
        return undefined;
    }
    return url;
}
function getNonSpecificUrlFromEnv(signalResourcePath) {
    const envUrl = getStringFromEnv('OTEL_EXPORTER_OTLP_ENDPOINT');
    if (envUrl === undefined) {
        return undefined;
    }
    return appendResourcePathToUrl(envUrl, signalResourcePath);
}
function getSpecificUrlFromEnv(signalIdentifier) {
    const envUrl = getStringFromEnv(`OTEL_EXPORTER_OTLP_${signalIdentifier}_ENDPOINT`);
    if (envUrl === undefined) {
        return undefined;
    }
    return appendRootPathToUrlIfNeeded(envUrl);
}
function readFileFromEnv(signalSpecificEnvVar, nonSignalSpecificEnvVar, warningMessage) {
    const signalSpecificPath = getStringFromEnv(signalSpecificEnvVar);
    const nonSignalSpecificPath = getStringFromEnv(nonSignalSpecificEnvVar);
    const filePath = signalSpecificPath ?? nonSignalSpecificPath;
    if (filePath != null) {
        try {
            return external_fs_.readFileSync(external_path_.resolve(process.cwd(), filePath));
        }
        catch {
            diag_api_diag.warn(warningMessage);
            return undefined;
        }
    }
    else {
        return undefined;
    }
}
function getClientCertificateFromEnv(signalIdentifier) {
    return readFileFromEnv(`OTEL_EXPORTER_OTLP_${signalIdentifier}_CLIENT_CERTIFICATE`, 'OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE', 'Failed to read client certificate chain file');
}
function getClientKeyFromEnv(signalIdentifier) {
    return readFileFromEnv(`OTEL_EXPORTER_OTLP_${signalIdentifier}_CLIENT_KEY`, 'OTEL_EXPORTER_OTLP_CLIENT_KEY', 'Failed to read client certificate private key file');
}
function getRootCertificateFromEnv(signalIdentifier) {
    return readFileFromEnv(`OTEL_EXPORTER_OTLP_${signalIdentifier}_CERTIFICATE`, 'OTEL_EXPORTER_OTLP_CERTIFICATE', 'Failed to read root certificate file');
}
/**
 * Reads and returns configuration from the environment
 *
 * @param signalIdentifier all caps part in environment variables that identifies the signal (e.g.: METRICS, TRACES, LOGS)
 * @param signalResourcePath signal resource path to append if necessary (e.g.: v1/metrics, v1/traces, v1/logs)
 */
function getNodeHttpConfigurationFromEnvironment(signalIdentifier, signalResourcePath) {
    return {
        ...getSharedConfigurationFromEnvironment(signalIdentifier),
        url: getSpecificUrlFromEnv(signalIdentifier) ??
            getNonSpecificUrlFromEnv(signalResourcePath),
        headers: wrapStaticHeadersInFunction(getStaticHeadersFromEnv(signalIdentifier)),
        agentFactory: httpAgentFactoryFromOptions({
            keepAlive: true,
            ca: getRootCertificateFromEnv(signalIdentifier),
            cert: getClientCertificateFromEnv(signalIdentifier),
            key: getClientKeyFromEnv(signalIdentifier),
        }),
    };
}
//# sourceMappingURL=otlp-node-http-env-configuration.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+otlp-exporter-base@0.222.0+e40b0dfdd726a224/node_modules/@opentelemetry/otlp-exporter-base/build/esm/configuration/convert-legacy-http-options.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */

function convertLegacyHeaders(config) {
    if (typeof config.headers === 'function') {
        return config.headers;
    }
    return wrapStaticHeadersInFunction(config.headers);
}
//# sourceMappingURL=convert-legacy-http-options.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+otlp-exporter-base@0.222.0+e40b0dfdd726a224/node_modules/@opentelemetry/otlp-exporter-base/build/esm/configuration/convert-legacy-node-http-options.js





function convertLegacyAgentOptions(config) {
    if (typeof config.httpAgentOptions === 'function') {
        return config.httpAgentOptions;
    }
    let legacy = config.httpAgentOptions;
    if (config.keepAlive != null) {
        legacy = { keepAlive: config.keepAlive, ...legacy };
    }
    if (legacy != null) {
        return httpAgentFactoryFromOptions(legacy);
    }
    else {
        return undefined;
    }
}
/**
 * @deprecated this will be removed in 2.0
 * @param config
 * @param signalIdentifier
 * @param signalResourcePath
 * @param requiredHeaders
 */
function convertLegacyHttpOptions(config, signalIdentifier, signalResourcePath, requiredHeaders) {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    if (config.metadata) {
        diag_api_diag.warn('Metadata cannot be set when using http');
    }
    return mergeOtlpNodeHttpConfigurationWithDefaults({
        url: config.url,
        headers: convertLegacyHeaders(config),
        concurrencyLimit: config.concurrencyLimit,
        timeoutMillis: config.timeoutMillis,
        compression: config.compression,
        agentFactory: convertLegacyAgentOptions(config),
        userAgent: config.userAgent,
    }, getNodeHttpConfigurationFromEnvironment(signalIdentifier, signalResourcePath), getNodeHttpConfigurationDefaults(requiredHeaders, signalResourcePath));
}
//# sourceMappingURL=convert-legacy-node-http-options.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+exporter-trace-otlp-http@0.222.0+e40b0dfdd726a224/node_modules/@opentelemetry/exporter-trace-otlp-http/build/esm/semconv.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
/*
 * This file contains a copy of unstable semantic convention definitions
 * used by this package.
 * @see https://github.com/open-telemetry/opentelemetry-js/tree/main/semantic-conventions#unstable-semconv
 */
/**
 * Enum value "otlp_http_span_exporter" for attribute {@link ATTR_OTEL_COMPONENT_TYPE}.
 *
 * OTLP span exporter over HTTP with protobuf serialization
 *
 * @experimental This enum value is experimental and is subject to breaking changes in minor releases of `@opentelemetry/semantic-conventions`.
 */
const OTEL_COMPONENT_TYPE_VALUE_OTLP_HTTP_SPAN_EXPORTER = 'otlp_http_span_exporter';
//# sourceMappingURL=semconv.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+exporter-trace-otlp-http@0.222.0+e40b0dfdd726a224/node_modules/@opentelemetry/exporter-trace-otlp-http/build/esm/platform/node/OTLPTraceExporter.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */




/**
 * Collector Trace Exporter for Node
 */
class OTLPTraceExporter extends OTLPExporterBase {
    constructor(config = {}) {
        super(createOtlpHttpExportDelegate(convertLegacyHttpOptions(config, 'TRACES', 'v1/traces', {
            'Content-Type': 'application/json',
        }), JsonTraceSerializer, OTEL_COMPONENT_TYPE_VALUE_OTLP_HTTP_SPAN_EXPORTER, TraceExporterMetricsHelper, config.selfObsMeterProvider));
    }
}
//# sourceMappingURL=OTLPTraceExporter.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+core@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/core/build/esm/version.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
// this is autogenerated file, see scripts/version-update.js
const build_esm_version_VERSION = '2.11.0';
//# sourceMappingURL=version.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+core@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/core/build/esm/semconv.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
/*
 * This file contains a copy of unstable semantic convention definitions
 * used by this package.
 * @see https://github.com/open-telemetry/opentelemetry-js/tree/main/semantic-conventions#unstable-semconv
 */
/**
 * The name of the runtime of this process.
 *
 * @example OpenJDK Runtime Environment
 *
 * @experimental This attribute is experimental and is subject to breaking changes in minor releases of `@opentelemetry/semantic-conventions`.
 */
const ATTR_PROCESS_RUNTIME_NAME = 'process.runtime.name';
//# sourceMappingURL=semconv.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+core@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/core/build/esm/platform/node/sdk-info.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */



/** Constants describing the SDK in use */
const SDK_INFO = {
    [ATTR_TELEMETRY_SDK_NAME]: 'opentelemetry',
    [ATTR_PROCESS_RUNTIME_NAME]: 'node',
    [ATTR_TELEMETRY_SDK_LANGUAGE]: TELEMETRY_SDK_LANGUAGE_VALUE_NODEJS,
    [ATTR_TELEMETRY_SDK_VERSION]: build_esm_version_VERSION,
};
//# sourceMappingURL=sdk-info.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+resources@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/resources/build/esm/default-service-name.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
let serviceName;
/**
 * Returns the default service name for OpenTelemetry resources.
 * In Node.js environments, returns "unknown_service:<process.argv0>".
 * In browser/edge environments, returns "unknown_service".
 */
function defaultServiceName() {
    if (serviceName === undefined) {
        try {
            const argv0 = globalThis.process.argv0;
            serviceName = argv0 ? `unknown_service:${argv0}` : 'unknown_service';
        }
        catch {
            serviceName = 'unknown_service';
        }
    }
    return serviceName;
}
/** @internal For testing purposes only */
function _clearDefaultServiceNameCache() {
    serviceName = undefined;
}
//# sourceMappingURL=default-service-name.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+resources@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/resources/build/esm/utils.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
const isPromiseLike = (val) => {
    return (val !== null &&
        typeof val === 'object' &&
        typeof val.then === 'function');
};
//# sourceMappingURL=utils.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+resources@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/resources/build/esm/ResourceImpl.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */





class ResourceImpl {
    _rawAttributes;
    _asyncAttributesPending = false;
    _schemaUrl;
    _memoizedAttributes;
    static FromAttributeList(attributes, options) {
        const res = new ResourceImpl({}, options);
        res._rawAttributes = guardedRawAttributes(attributes);
        res._asyncAttributesPending =
            attributes.filter(([_, val]) => isPromiseLike(val)).length > 0;
        return res;
    }
    constructor(
    /**
     * A dictionary of attributes with string keys and values that provide
     * information about the entity as numbers, strings or booleans
     * TODO: Consider to add check/validation on attributes.
     */
    resource, options) {
        const attributes = resource.attributes ?? {};
        this._rawAttributes = Object.entries(attributes).map(([k, v]) => {
            if (isPromiseLike(v)) {
                // side-effect
                this._asyncAttributesPending = true;
            }
            return [k, v];
        });
        this._rawAttributes = guardedRawAttributes(this._rawAttributes);
        this._schemaUrl = validateSchemaUrl(options?.schemaUrl);
    }
    get asyncAttributesPending() {
        return this._asyncAttributesPending;
    }
    async waitForAsyncAttributes() {
        if (!this.asyncAttributesPending) {
            return;
        }
        for (let i = 0; i < this._rawAttributes.length; i++) {
            const [k, v] = this._rawAttributes[i];
            this._rawAttributes[i] = [k, isPromiseLike(v) ? await v : v];
        }
        this._asyncAttributesPending = false;
    }
    get attributes() {
        if (this.asyncAttributesPending) {
            diag_api_diag.error('Accessing resource attributes before async attributes settled');
        }
        if (this._memoizedAttributes) {
            return this._memoizedAttributes;
        }
        const attrs = {};
        for (const [k, v] of this._rawAttributes) {
            if (isPromiseLike(v)) {
                diag_api_diag.debug(`Unsettled resource attribute ${k} skipped`);
                continue;
            }
            if (v != null) {
                attrs[k] ??= v;
            }
        }
        // only memoize output if all attributes are settled
        if (!this._asyncAttributesPending) {
            this._memoizedAttributes = attrs;
        }
        return attrs;
    }
    getRawAttributes() {
        return this._rawAttributes;
    }
    get schemaUrl() {
        return this._schemaUrl;
    }
    merge(resource) {
        if (resource == null)
            return this;
        // Order is important
        // Spec states incoming attributes override existing attributes
        const mergedSchemaUrl = mergeSchemaUrl(this, resource);
        const mergedOptions = mergedSchemaUrl
            ? { schemaUrl: mergedSchemaUrl }
            : undefined;
        return ResourceImpl.FromAttributeList([...resource.getRawAttributes(), ...this.getRawAttributes()], mergedOptions);
    }
}
function resourceFromAttributes(attributes, options) {
    return ResourceImpl.FromAttributeList(Object.entries(attributes), options);
}
function resourceFromDetectedResource(detectedResource, options) {
    return new ResourceImpl(detectedResource, options);
}
function emptyResource() {
    return resourceFromAttributes({});
}
function defaultResource() {
    return resourceFromAttributes({
        [ATTR_SERVICE_NAME]: defaultServiceName(),
        [ATTR_TELEMETRY_SDK_LANGUAGE]: SDK_INFO[ATTR_TELEMETRY_SDK_LANGUAGE],
        [ATTR_TELEMETRY_SDK_NAME]: SDK_INFO[ATTR_TELEMETRY_SDK_NAME],
        [ATTR_TELEMETRY_SDK_VERSION]: SDK_INFO[ATTR_TELEMETRY_SDK_VERSION],
    });
}
function guardedRawAttributes(attributes) {
    return attributes.map(([k, v]) => {
        if (isPromiseLike(v)) {
            return [
                k,
                v.catch(err => {
                    diag_api_diag.debug('promise rejection for resource attribute: %s - %s', k, err);
                    return undefined;
                }),
            ];
        }
        return [k, v];
    });
}
function validateSchemaUrl(schemaUrl) {
    if (typeof schemaUrl === 'string' || schemaUrl === undefined) {
        return schemaUrl;
    }
    diag_api_diag.warn('Schema URL must be string or undefined, got %s. Schema URL will be ignored.', schemaUrl);
    return undefined;
}
function mergeSchemaUrl(old, updating) {
    const oldSchemaUrl = old?.schemaUrl;
    const updatingSchemaUrl = updating?.schemaUrl;
    const isOldEmpty = oldSchemaUrl === undefined || oldSchemaUrl === '';
    const isUpdatingEmpty = updatingSchemaUrl === undefined || updatingSchemaUrl === '';
    if (isOldEmpty) {
        return updatingSchemaUrl;
    }
    if (isUpdatingEmpty) {
        return oldSchemaUrl;
    }
    if (oldSchemaUrl === updatingSchemaUrl) {
        return oldSchemaUrl;
    }
    diag_api_diag.warn('Schema URL merge conflict: old resource has "%s", updating resource has "%s". Resulting resource will have undefined Schema URL.', oldSchemaUrl, updatingSchemaUrl);
    return undefined;
}
//# sourceMappingURL=ResourceImpl.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+core@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/core/build/esm/utils/lodash.merge.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
/* eslint-disable @typescript-eslint/no-explicit-any */
/**
 * based on lodash in order to support esm builds without esModuleInterop.
 * lodash is using MIT License.
 **/
const objectTag = '[object Object]';
const nullTag = '[object Null]';
const undefinedTag = '[object Undefined]';
const funcProto = Function.prototype;
const funcToString = funcProto.toString;
const objectCtorString = funcToString.call(Object);
const getPrototypeOf = Object.getPrototypeOf;
const objectProto = Object.prototype;
const lodash_merge_hasOwnProperty = objectProto.hasOwnProperty;
const symToStringTag = Symbol ? Symbol.toStringTag : undefined;
const nativeObjectToString = objectProto.toString;
/**
 * Checks if `value` is a plain object, that is, an object created by the
 * `Object` constructor or one with a `[[Prototype]]` of `null`.
 *
 * @static
 * @memberOf _
 * @since 0.8.0
 * @category Lang
 * @param {*} value The value to check.
 * @returns {boolean} Returns `true` if `value` is a plain object, else `false`.
 * @example
 *
 * function Foo() {
 *   this.a = 1;
 * }
 *
 * _.isPlainObject(new Foo);
 * // => false
 *
 * _.isPlainObject([1, 2, 3]);
 * // => false
 *
 * _.isPlainObject({ 'x': 0, 'y': 0 });
 * // => true
 *
 * _.isPlainObject(Object.create(null));
 * // => true
 */
function isPlainObject(value) {
    if (!isObjectLike(value) || baseGetTag(value) !== objectTag) {
        return false;
    }
    const proto = getPrototypeOf(value);
    if (proto === null) {
        return true;
    }
    const Ctor = lodash_merge_hasOwnProperty.call(proto, 'constructor') && proto.constructor;
    return (typeof Ctor == 'function' &&
        Ctor instanceof Ctor &&
        funcToString.call(Ctor) === objectCtorString);
}
/**
 * Checks if `value` is object-like. A value is object-like if it's not `null`
 * and has a `typeof` result of "object".
 *
 * @static
 * @memberOf _
 * @since 4.0.0
 * @category Lang
 * @param {*} value The value to check.
 * @returns {boolean} Returns `true` if `value` is object-like, else `false`.
 * @example
 *
 * _.isObjectLike({});
 * // => true
 *
 * _.isObjectLike([1, 2, 3]);
 * // => true
 *
 * _.isObjectLike(_.noop);
 * // => false
 *
 * _.isObjectLike(null);
 * // => false
 */
function isObjectLike(value) {
    return value != null && typeof value == 'object';
}
/**
 * The base implementation of `getTag` without fallbacks for buggy environments.
 *
 * @private
 * @param {*} value The value to query.
 * @returns {string} Returns the `toStringTag`.
 */
function baseGetTag(value) {
    if (value == null) {
        return value === undefined ? undefinedTag : nullTag;
    }
    return symToStringTag && symToStringTag in Object(value)
        ? getRawTag(value)
        : objectToString(value);
}
/**
 * A specialized version of `baseGetTag` which ignores `Symbol.toStringTag` values.
 *
 * @private
 * @param {*} value The value to query.
 * @returns {string} Returns the raw `toStringTag`.
 */
function getRawTag(value) {
    const isOwn = lodash_merge_hasOwnProperty.call(value, symToStringTag), tag = value[symToStringTag];
    let unmasked = false;
    try {
        value[symToStringTag] = undefined;
        unmasked = true;
    }
    catch {
        // silence
    }
    const result = nativeObjectToString.call(value);
    if (unmasked) {
        if (isOwn) {
            value[symToStringTag] = tag;
        }
        else {
            delete value[symToStringTag];
        }
    }
    return result;
}
/**
 * Converts `value` to a string using `Object.prototype.toString`.
 *
 * @private
 * @param {*} value The value to convert.
 * @returns {string} Returns the converted string.
 */
function objectToString(value) {
    return nativeObjectToString.call(value);
}
//# sourceMappingURL=lodash.merge.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+core@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/core/build/esm/utils/merge.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
/* eslint-disable @typescript-eslint/no-explicit-any */

const MAX_LEVEL = 20;
/**
 * Merges objects together
 * @param args - objects / values to be merged
 */
function merge(...args) {
    let result = args.shift();
    const objects = new WeakMap();
    while (args.length > 0) {
        result = mergeTwoObjects(result, args.shift(), 0, objects);
    }
    return result;
}
function takeValue(value) {
    if (isArray(value)) {
        return value.slice();
    }
    return value;
}
/**
 * Merges two objects
 * @param one - first object
 * @param two - second object
 * @param level - current deep level
 * @param objects - objects holder that has been already referenced - to prevent
 * cyclic dependency
 */
function mergeTwoObjects(one, two, level = 0, objects) {
    let result;
    if (level > MAX_LEVEL) {
        return undefined;
    }
    level++;
    if (isPrimitive(one) || isPrimitive(two) || isFunction(two)) {
        result = takeValue(two);
    }
    else if (isArray(one)) {
        result = one.slice();
        if (isArray(two)) {
            for (let i = 0, j = two.length; i < j; i++) {
                result.push(takeValue(two[i]));
            }
        }
        else if (isObject(two)) {
            const keys = Object.keys(two);
            for (let i = 0, j = keys.length; i < j; i++) {
                const key = keys[i];
                if (key === '__proto__' ||
                    key === 'constructor' ||
                    key === 'prototype') {
                    continue;
                }
                result[key] = takeValue(two[key]);
            }
        }
    }
    else if (isObject(one)) {
        if (isObject(two)) {
            if (!shouldMerge(one, two)) {
                return two;
            }
            result = Object.assign({}, one);
            const keys = Object.keys(two);
            for (let i = 0, j = keys.length; i < j; i++) {
                const key = keys[i];
                if (key === '__proto__' ||
                    key === 'constructor' ||
                    key === 'prototype') {
                    continue;
                }
                const twoValue = two[key];
                if (isPrimitive(twoValue)) {
                    if (typeof twoValue === 'undefined') {
                        delete result[key];
                    }
                    else {
                        // result[key] = takeValue(twoValue);
                        result[key] = twoValue;
                    }
                }
                else {
                    const obj1 = result[key];
                    const obj2 = twoValue;
                    if (wasObjectReferenced(one, key, objects) ||
                        wasObjectReferenced(two, key, objects)) {
                        delete result[key];
                    }
                    else {
                        if (isObject(obj1) && isObject(obj2)) {
                            const arr1 = objects.get(obj1) || [];
                            const arr2 = objects.get(obj2) || [];
                            arr1.push({ obj: one, key });
                            arr2.push({ obj: two, key });
                            objects.set(obj1, arr1);
                            objects.set(obj2, arr2);
                        }
                        result[key] = mergeTwoObjects(result[key], twoValue, level, objects);
                    }
                }
            }
        }
        else {
            result = two;
        }
    }
    return result;
}
/**
 * Function to check if object has been already reference
 * @param obj
 * @param key
 * @param objects
 */
function wasObjectReferenced(obj, key, objects) {
    const arr = objects.get(obj[key]) || [];
    for (let i = 0, j = arr.length; i < j; i++) {
        const info = arr[i];
        if (info.key === key && info.obj === obj) {
            return true;
        }
    }
    return false;
}
function isArray(value) {
    return Array.isArray(value);
}
function isFunction(value) {
    return typeof value === 'function';
}
function isObject(value) {
    return (!isPrimitive(value) &&
        !isArray(value) &&
        !isFunction(value) &&
        typeof value === 'object');
}
function isPrimitive(value) {
    return (typeof value === 'string' ||
        typeof value === 'number' ||
        typeof value === 'boolean' ||
        typeof value === 'undefined' ||
        value instanceof Date ||
        value instanceof RegExp ||
        value === null);
}
function shouldMerge(one, two) {
    if (!isPlainObject(one) || !isPlainObject(two)) {
        return false;
    }
    return true;
}
//# sourceMappingURL=merge.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+sdk-trace@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/sdk-trace/build/esm/Sampler.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
/**
 * A sampling decision that determines how a {@link Span} will be recorded
 * and collected.
 */
var SamplingDecision;
(function (SamplingDecision) {
    /**
     * `Span.isRecording() === false`, span will not be recorded and all events
     * and attributes will be dropped.
     */
    SamplingDecision[SamplingDecision["NOT_RECORD"] = 0] = "NOT_RECORD";
    /**
     * `Span.isRecording() === true`, but `Sampled` flag in {@link TraceFlags}
     * MUST NOT be set.
     */
    SamplingDecision[SamplingDecision["RECORD"] = 1] = "RECORD";
    /**
     * `Span.isRecording() === true` AND `Sampled` flag in {@link TraceFlags}
     * MUST be set.
     */
    SamplingDecision[SamplingDecision["RECORD_AND_SAMPLED"] = 2] = "RECORD_AND_SAMPLED";
})(SamplingDecision || (SamplingDecision = {}));
//# sourceMappingURL=Sampler.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+sdk-trace@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/sdk-trace/build/esm/sampler/AlwaysOnSampler.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */

/** Sampler that samples all traces. */
class AlwaysOnSampler {
    shouldSample() {
        return {
            decision: SamplingDecision.RECORD_AND_SAMPLED,
        };
    }
    toString() {
        return 'AlwaysOnSampler';
    }
}
//# sourceMappingURL=AlwaysOnSampler.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+sdk-trace@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/sdk-trace/build/esm/sampler/AlwaysOffSampler.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */

/** Sampler that samples no traces. */
class AlwaysOffSampler {
    shouldSample() {
        return {
            decision: SamplingDecision.NOT_RECORD,
        };
    }
    toString() {
        return 'AlwaysOffSampler';
    }
}
//# sourceMappingURL=AlwaysOffSampler.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+core@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/core/build/esm/common/logging-error-handler.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */

/**
 * Returns a function that logs an error using the provided logger, or a
 * console logger if one was not provided.
 */
function loggingErrorHandler() {
    return (ex) => {
        diag_api_diag.error(stringifyException(ex));
    };
}
/**
 * Converts an exception into a string representation
 * @param {Exception} ex
 */
function stringifyException(ex) {
    if (typeof ex === 'string') {
        return ex;
    }
    else {
        return JSON.stringify(flattenException(ex));
    }
}
/**
 * Flattens an exception into key-value pairs by traversing the prototype chain
 * and coercing values to strings. Duplicate properties will not be overwritten;
 * the first insert wins.
 */
function flattenException(ex) {
    const result = {};
    let current = ex;
    while (current !== null) {
        Object.getOwnPropertyNames(current).forEach(propertyName => {
            if (result[propertyName])
                return;
            const value = current[propertyName];
            if (value) {
                result[propertyName] = String(value);
            }
        });
        current = Object.getPrototypeOf(current);
    }
    return result;
}
//# sourceMappingURL=logging-error-handler.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+core@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/core/build/esm/common/global-error-handler.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */

/** The global error handler delegate */
let delegateHandler = loggingErrorHandler();
/**
 * Set the global error handler
 * @param {ErrorHandler} handler
 */
function setGlobalErrorHandler(handler) {
    delegateHandler = handler;
}
/**
 * Return the global error handler
 * @param {Exception} ex
 */
function globalErrorHandler(ex) {
    try {
        delegateHandler(ex);
    }
    catch { } // eslint-disable-line no-empty
}
//# sourceMappingURL=global-error-handler.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+sdk-trace@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/sdk-trace/build/esm/sampler/ParentBasedSampler.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */




/**
 * A composite sampler that either respects the parent span's sampling decision
 * or delegates to `delegateSampler` for root spans.
 */
class ParentBasedSampler {
    _root;
    _remoteParentSampled;
    _remoteParentNotSampled;
    _localParentSampled;
    _localParentNotSampled;
    constructor(config) {
        this._root = config.root;
        if (!this._root) {
            globalErrorHandler(new Error('ParentBasedSampler must have a root sampler configured'));
            this._root = new AlwaysOnSampler();
        }
        this._remoteParentSampled =
            config.remoteParentSampled ?? new AlwaysOnSampler();
        this._remoteParentNotSampled =
            config.remoteParentNotSampled ?? new AlwaysOffSampler();
        this._localParentSampled =
            config.localParentSampled ?? new AlwaysOnSampler();
        this._localParentNotSampled =
            config.localParentNotSampled ?? new AlwaysOffSampler();
    }
    shouldSample(context, traceId, spanName, spanKind, attributes, links) {
        const parentContext = trace_api/* trace */.u.getSpanContext(context);
        if (!parentContext || !(0,spancontext_utils/* isSpanContextValid */.YA)(parentContext)) {
            return this._root.shouldSample(context, traceId, spanName, spanKind, attributes, links);
        }
        if (parentContext.isRemote) {
            if (parentContext.traceFlags & trace_flags/* TraceFlags */.X.SAMPLED) {
                return this._remoteParentSampled.shouldSample(context, traceId, spanName, spanKind, attributes, links);
            }
            return this._remoteParentNotSampled.shouldSample(context, traceId, spanName, spanKind, attributes, links);
        }
        if (parentContext.traceFlags & trace_flags/* TraceFlags */.X.SAMPLED) {
            return this._localParentSampled.shouldSample(context, traceId, spanName, spanKind, attributes, links);
        }
        return this._localParentNotSampled.shouldSample(context, traceId, spanName, spanKind, attributes, links);
    }
    toString() {
        return `ParentBased{root=${this._root.toString()}, remoteParentSampled=${this._remoteParentSampled.toString()}, remoteParentNotSampled=${this._remoteParentNotSampled.toString()}, localParentSampled=${this._localParentSampled.toString()}, localParentNotSampled=${this._localParentNotSampled.toString()}}`;
    }
}
//# sourceMappingURL=ParentBasedSampler.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+sdk-trace@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/sdk-trace/build/esm/sampler/TraceIdRatioBasedSampler.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */


/** Sampler that samples a given fraction of traces based of trace id deterministically. */
class TraceIdRatioBasedSampler {
    _ratio;
    _upperBound;
    constructor(ratio = 0) {
        this._ratio = this._normalize(ratio);
        this._upperBound =
            this._ratio === 1 ? 0x100000000 : Math.floor(this._ratio * 0xffffffff);
    }
    shouldSample(context, traceId) {
        return {
            decision: (0,spancontext_utils/* isValidTraceId */.hX)(traceId) && this._accumulate(traceId) < this._upperBound
                ? SamplingDecision.RECORD_AND_SAMPLED
                : SamplingDecision.NOT_RECORD,
        };
    }
    toString() {
        return `TraceIdRatioBased{${this._ratio}}`;
    }
    _normalize(ratio) {
        if (typeof ratio !== 'number' || isNaN(ratio))
            return 0;
        return ratio >= 1 ? 1 : ratio <= 0 ? 0 : ratio;
    }
    _accumulate(traceId) {
        let accumulation = 0;
        for (let i = 0; i < 32; i += 8) {
            let part = 0;
            for (let j = 0; j < 8; j++) {
                const c = traceId.charCodeAt(i + j);
                // Convert hex char code to value: '0'-'9' -> 0-9, 'a'-'f' -> 10-15, 'A'-'F' -> 10-15
                const v = c < 58 ? c - 48 : c < 71 ? c - 55 : c - 87;
                part = (part << 4) | v;
            }
            accumulation = (accumulation ^ part) >>> 0;
        }
        return accumulation;
    }
}
//# sourceMappingURL=TraceIdRatioBasedSampler.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+sdk-trace-base@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/sdk-trace-base/build/esm/config.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */



var TracesSamplerValues;
(function (TracesSamplerValues) {
    TracesSamplerValues["AlwaysOff"] = "always_off";
    TracesSamplerValues["AlwaysOn"] = "always_on";
    TracesSamplerValues["ParentBasedAlwaysOff"] = "parentbased_always_off";
    TracesSamplerValues["ParentBasedAlwaysOn"] = "parentbased_always_on";
    TracesSamplerValues["ParentBasedTraceIdRatio"] = "parentbased_traceidratio";
    TracesSamplerValues["TraceIdRatio"] = "traceidratio";
})(TracesSamplerValues || (TracesSamplerValues = {}));
const DEFAULT_RATIO = 1;
/**
 * Load default configuration. For fields with primitive values, any user-provided
 * value will override the corresponding default value. For fields with
 * non-primitive values (like `spanLimits`), the user-provided value will be
 * used to extend the default value.
 */
// object needs to be wrapped in this function and called when needed otherwise
// envs are parsed before tests are ran - causes tests using these envs to fail
function loadDefaultConfig() {
    return {
        sampler: buildSamplerFromEnv(),
        forceFlushTimeoutMillis: 30000,
        generalLimits: {
            attributeValueLengthLimit: getNumberFromEnv('OTEL_ATTRIBUTE_VALUE_LENGTH_LIMIT') ?? Infinity,
            attributeCountLimit: getNumberFromEnv('OTEL_ATTRIBUTE_COUNT_LIMIT') ?? 128,
        },
        spanLimits: {
            attributeValueLengthLimit: getNumberFromEnv('OTEL_SPAN_ATTRIBUTE_VALUE_LENGTH_LIMIT') ?? Infinity,
            attributeCountLimit: getNumberFromEnv('OTEL_SPAN_ATTRIBUTE_COUNT_LIMIT') ?? 128,
            linkCountLimit: getNumberFromEnv('OTEL_SPAN_LINK_COUNT_LIMIT') ?? 128,
            eventCountLimit: getNumberFromEnv('OTEL_SPAN_EVENT_COUNT_LIMIT') ?? 128,
            attributePerEventCountLimit: getNumberFromEnv('OTEL_SPAN_ATTRIBUTE_PER_EVENT_COUNT_LIMIT') ?? 128,
            attributePerLinkCountLimit: getNumberFromEnv('OTEL_SPAN_ATTRIBUTE_PER_LINK_COUNT_LIMIT') ?? 128,
        },
    };
}
/**
 * Based on environment, builds a sampler, complies with specification.
 */
function buildSamplerFromEnv() {
    const sampler = getStringFromEnv('OTEL_TRACES_SAMPLER') ??
        TracesSamplerValues.ParentBasedAlwaysOn;
    switch (sampler) {
        case TracesSamplerValues.AlwaysOn:
            return new AlwaysOnSampler();
        case TracesSamplerValues.AlwaysOff:
            return new AlwaysOffSampler();
        case TracesSamplerValues.ParentBasedAlwaysOn:
            return new ParentBasedSampler({
                root: new AlwaysOnSampler(),
            });
        case TracesSamplerValues.ParentBasedAlwaysOff:
            return new ParentBasedSampler({
                root: new AlwaysOffSampler(),
            });
        case TracesSamplerValues.TraceIdRatio:
            return new TraceIdRatioBasedSampler(getSamplerProbabilityFromEnv());
        case TracesSamplerValues.ParentBasedTraceIdRatio:
            return new ParentBasedSampler({
                root: new TraceIdRatioBasedSampler(getSamplerProbabilityFromEnv()),
            });
        default:
            diag_api_diag.error(`OTEL_TRACES_SAMPLER value "${sampler}" invalid, defaulting to "${TracesSamplerValues.ParentBasedAlwaysOn}".`);
            return new ParentBasedSampler({
                root: new AlwaysOnSampler(),
            });
    }
}
function getSamplerProbabilityFromEnv() {
    const probability = getNumberFromEnv('OTEL_TRACES_SAMPLER_ARG');
    if (probability == null) {
        diag_api_diag.error(`OTEL_TRACES_SAMPLER_ARG is blank, defaulting to ${DEFAULT_RATIO}.`);
        return DEFAULT_RATIO;
    }
    if (probability < 0 || probability > 1) {
        diag_api_diag.error(`OTEL_TRACES_SAMPLER_ARG=${probability} was given, but it is out of range ([0..1]), defaulting to ${DEFAULT_RATIO}.`);
        return DEFAULT_RATIO;
    }
    return probability;
}
//# sourceMappingURL=config.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+sdk-trace-base@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/sdk-trace-base/build/esm/utility.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */

const DEFAULT_ATTRIBUTE_COUNT_LIMIT = 128;
const DEFAULT_ATTRIBUTE_VALUE_LENGTH_LIMIT = Infinity;
/**
 * When general limits are provided and model specific limits are not,
 * configures the model specific limits by using the values from the general ones.
 * @param userConfig User provided tracer configuration
 */
function reconfigureLimits(userConfig) {
    const spanLimits = Object.assign({}, userConfig.spanLimits);
    /**
     * Reassign span attribute count limit to use first non null value defined by user or use default value
     */
    spanLimits.attributeCountLimit =
        userConfig.spanLimits?.attributeCountLimit ??
            userConfig.generalLimits?.attributeCountLimit ??
            getNumberFromEnv('OTEL_SPAN_ATTRIBUTE_COUNT_LIMIT') ??
            getNumberFromEnv('OTEL_ATTRIBUTE_COUNT_LIMIT') ??
            DEFAULT_ATTRIBUTE_COUNT_LIMIT;
    /**
     * Reassign span attribute value length limit to use first non null value defined by user or use default value
     */
    spanLimits.attributeValueLengthLimit =
        userConfig.spanLimits?.attributeValueLengthLimit ??
            userConfig.generalLimits?.attributeValueLengthLimit ??
            getNumberFromEnv('OTEL_SPAN_ATTRIBUTE_VALUE_LENGTH_LIMIT') ??
            getNumberFromEnv('OTEL_ATTRIBUTE_VALUE_LENGTH_LIMIT') ??
            DEFAULT_ATTRIBUTE_VALUE_LENGTH_LIMIT;
    return Object.assign({}, userConfig, { spanLimits });
}
//# sourceMappingURL=utility.js.map
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api@1.9.1/node_modules/@opentelemetry/api/build/esm/trace/invalid-span-constants.js
var invalid_span_constants = __webpack_require__(37957);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api@1.9.1/node_modules/@opentelemetry/api/build/esm/trace/SamplingResult.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
/**
 * @deprecated use the one declared in @opentelemetry/sdk-trace-base instead.
 * A sampling decision that determines how a {@link Span} will be recorded
 * and collected.
 *
 * @since 1.0.0
 */
var SamplingResult_SamplingDecision;
(function (SamplingDecision) {
    /**
     * `Span.isRecording() === false`, span will not be recorded and all events
     * and attributes will be dropped.
     */
    SamplingDecision[SamplingDecision["NOT_RECORD"] = 0] = "NOT_RECORD";
    /**
     * `Span.isRecording() === true`, but `Sampled` flag in {@link TraceFlags}
     * MUST NOT be set.
     */
    SamplingDecision[SamplingDecision["RECORD"] = 1] = "RECORD";
    /**
     * `Span.isRecording() === true` AND `Sampled` flag in {@link TraceFlags}
     * MUST be set.
     */
    SamplingDecision[SamplingDecision["RECORD_AND_SAMPLED"] = 2] = "RECORD_AND_SAMPLED";
})(SamplingResult_SamplingDecision || (SamplingResult_SamplingDecision = {}));
//# sourceMappingURL=SamplingResult.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+core@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/core/build/esm/common/attributes.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */

function sanitizeAttributes(attributes) {
    const out = {};
    if (typeof attributes !== 'object' || attributes == null) {
        return out;
    }
    for (const key in attributes) {
        if (!Object.prototype.hasOwnProperty.call(attributes, key)) {
            continue;
        }
        if (!isAttributeKey(key)) {
            diag_api_diag.warn(`Invalid attribute key: ${key}`);
            continue;
        }
        const val = attributes[key];
        if (!isAttributeValue(val)) {
            diag_api_diag.warn(`Invalid attribute value set for key: ${key}`);
            continue;
        }
        if (Array.isArray(val)) {
            out[key] = val.slice();
        }
        else {
            out[key] = val;
        }
    }
    return out;
}
function isAttributeKey(key) {
    return typeof key === 'string' && key !== '';
}
function isAttributeValue(val) {
    if (val == null) {
        return true;
    }
    if (Array.isArray(val)) {
        return isHomogeneousAttributeValueArray(val);
    }
    return isValidPrimitiveAttributeValueType(typeof val);
}
function isHomogeneousAttributeValueArray(arr) {
    let type;
    for (const element of arr) {
        // null/undefined elements are allowed
        if (element == null)
            continue;
        const elementType = typeof element;
        if (elementType === type) {
            continue;
        }
        if (!type) {
            if (isValidPrimitiveAttributeValueType(elementType)) {
                type = elementType;
                continue;
            }
            // encountered an invalid primitive
            return false;
        }
        return false;
    }
    return true;
}
function isValidPrimitiveAttributeValueType(valType) {
    switch (valType) {
        case 'number':
        case 'boolean':
        case 'string':
            return true;
    }
    return false;
}
//# sourceMappingURL=attributes.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+sdk-trace@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/sdk-trace/build/esm/enums.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
// Event name definitions
const ExceptionEventName = 'exception';
//# sourceMappingURL=enums.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+sdk-trace@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/sdk-trace/build/esm/inspect.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
/**
 * Well-known symbol used by Node.js `util.inspect` (and `console.*`) to
 * render an object via a custom representation. Defined as a global Symbol
 * so it works without importing from `node:util`, keeping this module safe
 * for browser builds (where the symbol is simply never looked up).
 */
const inspectCustom = Symbol.for('nodejs.util.inspect.custom');
/**
 * Collect a Resource's settled attributes without touching the
 * `attributes` getter, which emits diag.error/debug entries when async
 * attribute detectors are still pending. Promise-like (unsettled)
 * entries are silently skipped so logging a Span/Tracer/Provider during
 * startup doesn't recurse through the diag pipeline.
 */
function settledResourceAttributes(resource) {
    const attrs = {};
    for (const [k, v] of resource.getRawAttributes()) {
        if (typeof v?.then === 'function') {
            continue;
        }
        if (v != null) {
            attrs[k] ??= v;
        }
    }
    return attrs;
}
/**
 * Build a class-tagged inspect representation. Returns a stub like
 * `[ClassName]` once the recursion budget is exhausted, otherwise returns
 * `ClassName <inspected payload>` so nested fields keep proper coloring,
 * indentation, and depth handling. In environments that don't supply an
 * `inspect` callback (e.g. browsers), falls back to returning the raw
 * payload object.
 */
function formatInspect(className, payload, depth, options, inspect) {
    if (typeof depth === 'number' && depth < 0) {
        const tag = `[${className}]`;
        return options?.stylize ? options.stylize(tag, 'special') : tag;
    }
    if (typeof inspect !== 'function' || !options) {
        return payload;
    }
    const childOptions = {
        ...options,
        depth: options.depth == null ? options.depth : options.depth - 1,
    };
    return `${className} ${inspect(payload, childOptions)}`;
}
//# sourceMappingURL=inspect.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+sdk-trace@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/sdk-trace/build/esm/Span.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */





/**
 * This class represents a span.
 */
class SpanImpl {
    // Below properties are included to implement ReadableSpan for export
    // purposes but are not intended to be written-to directly.
    _spanContext;
    kind;
    parentSpanContext;
    attributes = {};
    links = [];
    events = [];
    startTime;
    resource;
    instrumentationScope;
    _droppedAttributesCount = 0;
    _droppedEventsCount = 0;
    _droppedLinksCount = 0;
    _attributesCount = 0;
    name;
    status = {
        code: SpanStatusCode.UNSET,
    };
    endTime = [0, 0];
    _ended = false;
    _duration = [-1, -1];
    _spanProcessor;
    _spanLimits;
    _attributeValueLengthLimit;
    _recordEndMetrics;
    _performanceStartTime;
    _performanceOffset;
    _startTimeProvided;
    /**
     * Constructs a new SpanImpl instance.
     */
    constructor(opts) {
        const now = Date.now();
        this._spanContext = opts.spanContext;
        this._performanceStartTime = otperformance.now();
        this._performanceOffset =
            now - (this._performanceStartTime + otperformance.timeOrigin);
        this._startTimeProvided = opts.startTime != null;
        this._spanLimits = opts.spanLimits;
        this._attributeValueLengthLimit =
            this._spanLimits.attributeValueLengthLimit ?? 0;
        this._spanProcessor = opts.spanProcessor;
        this.name = opts.name;
        this.parentSpanContext = opts.parentSpanContext;
        this.kind = opts.kind;
        if (opts.links) {
            for (const link of opts.links) {
                this.addLink(link);
            }
        }
        this.startTime = this._getTime(opts.startTime ?? now);
        this.resource = opts.resource;
        this.instrumentationScope = opts.scope;
        this._recordEndMetrics = opts.recordEndMetrics;
        if (opts.attributes != null) {
            this.setAttributes(opts.attributes);
        }
        this._spanProcessor.onStart(this, opts.context);
    }
    spanContext() {
        return this._spanContext;
    }
    setAttribute(key, value) {
        if (value == null || this._isSpanEnded())
            return this;
        if (key.length === 0) {
            diag_api_diag.warn(`Invalid attribute key: ${key}`);
            return this;
        }
        if (!isAttributeValue(value)) {
            diag_api_diag.warn(`Invalid attribute value set for key: ${key}`);
            return this;
        }
        const { attributeCountLimit } = this._spanLimits;
        const isNewKey = !Object.prototype.hasOwnProperty.call(this.attributes, key);
        if (attributeCountLimit !== undefined &&
            this._attributesCount >= attributeCountLimit &&
            isNewKey) {
            this._droppedAttributesCount++;
            return this;
        }
        this.attributes[key] = this._truncateToSize(value);
        if (isNewKey) {
            this._attributesCount++;
        }
        return this;
    }
    setAttributes(attributes) {
        for (const key in attributes) {
            if (Object.prototype.hasOwnProperty.call(attributes, key)) {
                this.setAttribute(key, attributes[key]);
            }
        }
        return this;
    }
    /**
     *
     * @param name Span Name
     * @param [attributesOrStartTime] Span attributes or start time
     *     if type is {@type TimeInput} and 3rd param is undefined
     * @param [timeStamp] Specified time stamp for the event
     */
    addEvent(name, attributesOrStartTime, timeStamp) {
        if (this._isSpanEnded())
            return this;
        const { eventCountLimit } = this._spanLimits;
        if (eventCountLimit === 0) {
            diag_api_diag.warn('No events allowed.');
            this._droppedEventsCount++;
            return this;
        }
        if (eventCountLimit !== undefined &&
            this.events.length >= eventCountLimit) {
            if (this._droppedEventsCount === 0) {
                diag_api_diag.debug('Dropping extra events.');
            }
            this.events.shift();
            this._droppedEventsCount++;
        }
        if (isTimeInput(attributesOrStartTime)) {
            if (!isTimeInput(timeStamp)) {
                timeStamp = attributesOrStartTime;
            }
            attributesOrStartTime = undefined;
        }
        const sanitized = sanitizeAttributes(attributesOrStartTime);
        const { attributePerEventCountLimit } = this._spanLimits;
        const attributes = {};
        let droppedAttributesCount = 0;
        let eventAttributesCount = 0;
        for (const attr in sanitized) {
            if (!Object.prototype.hasOwnProperty.call(sanitized, attr)) {
                continue;
            }
            const attrVal = sanitized[attr];
            if (attributePerEventCountLimit !== undefined &&
                eventAttributesCount >= attributePerEventCountLimit) {
                droppedAttributesCount++;
                continue;
            }
            attributes[attr] = this._truncateToSize(attrVal);
            eventAttributesCount++;
        }
        this.events.push({
            name,
            attributes,
            time: this._getTime(timeStamp),
            droppedAttributesCount,
        });
        return this;
    }
    addLink(link) {
        if (this._isSpanEnded())
            return this;
        const { linkCountLimit } = this._spanLimits;
        if (linkCountLimit === 0) {
            this._droppedLinksCount++;
            return this;
        }
        if (linkCountLimit !== undefined && this.links.length >= linkCountLimit) {
            if (this._droppedLinksCount === 0) {
                diag_api_diag.debug('Dropping extra links.');
            }
            this.links.shift();
            this._droppedLinksCount++;
        }
        const { attributePerLinkCountLimit } = this._spanLimits;
        const sanitized = sanitizeAttributes(link.attributes);
        const attributes = {};
        let droppedAttributesCount = 0;
        let linkAttributesCount = 0;
        for (const attr in sanitized) {
            if (!Object.prototype.hasOwnProperty.call(sanitized, attr)) {
                continue;
            }
            const attrVal = sanitized[attr];
            if (attributePerLinkCountLimit !== undefined &&
                linkAttributesCount >= attributePerLinkCountLimit) {
                droppedAttributesCount++;
                continue;
            }
            attributes[attr] = this._truncateToSize(attrVal);
            linkAttributesCount++;
        }
        const processedLink = { context: link.context };
        if (linkAttributesCount > 0) {
            processedLink.attributes = attributes;
        }
        if (droppedAttributesCount > 0) {
            processedLink.droppedAttributesCount = droppedAttributesCount;
        }
        this.links.push(processedLink);
        return this;
    }
    addLinks(links) {
        for (const link of links) {
            this.addLink(link);
        }
        return this;
    }
    setStatus(status) {
        if (this._isSpanEnded())
            return this;
        if (status.code === SpanStatusCode.UNSET)
            return this;
        if (this.status.code === SpanStatusCode.OK)
            return this;
        const newStatus = { code: status.code };
        // When using try-catch, the caught "error" is of type `any`. When then assigning `any` to `status.message`,
        // TypeScript will not error. While this can happen during use of any API, it is more common on Span#setStatus()
        // as it's likely used in a catch-block. Therefore, we validate if `status.message` is actually a string, null, or
        // undefined to avoid an incorrect type causing issues downstream.
        if (status.code === SpanStatusCode.ERROR) {
            if (typeof status.message === 'string') {
                newStatus.message = status.message;
            }
            else if (status.message != null) {
                diag_api_diag.warn(`Dropping invalid status.message of type '${typeof status.message}', expected 'string'`);
            }
        }
        this.status = newStatus;
        return this;
    }
    updateName(name) {
        if (this._isSpanEnded())
            return this;
        this.name = name;
        return this;
    }
    end(endTime) {
        if (this._isSpanEnded()) {
            diag_api_diag.error(`${this.name} ${this._spanContext.traceId}-${this._spanContext.spanId} - You can only call end() on a span once.`);
            return;
        }
        this.endTime = this._getTime(endTime);
        this._duration = hrTimeDuration(this.startTime, this.endTime);
        if (this._duration[0] < 0) {
            diag_api_diag.warn('Inconsistent start and end time, startTime > endTime. Setting span duration to 0ms.', this.startTime, this.endTime);
            this.endTime = this.startTime.slice();
            this._duration = [0, 0];
        }
        if (this._droppedEventsCount > 0) {
            diag_api_diag.warn(`Dropped ${this._droppedEventsCount} events because eventCountLimit reached`);
        }
        if (this._droppedLinksCount > 0) {
            diag_api_diag.warn(`Dropped ${this._droppedLinksCount} links because linkCountLimit reached`);
        }
        if (this._spanProcessor.onEnding) {
            this._spanProcessor.onEnding(this);
        }
        this._recordEndMetrics?.();
        this._ended = true;
        this._spanProcessor.onEnd(this);
    }
    _getTime(inp) {
        if (typeof inp === 'number' && inp <= otperformance.now()) {
            // must be a performance timestamp
            // apply correction and convert to hrtime
            return hrTime(inp + this._performanceOffset);
        }
        if (typeof inp === 'number') {
            return millisToHrTime(inp);
        }
        if (inp instanceof Date) {
            return millisToHrTime(inp.getTime());
        }
        if (isTimeInputHrTime(inp)) {
            return inp;
        }
        if (this._startTimeProvided) {
            // if user provided a time for the start manually
            // we can't use duration to calculate event/end times
            return millisToHrTime(Date.now());
        }
        const msDuration = otperformance.now() - this._performanceStartTime;
        return addHrTimes(this.startTime, millisToHrTime(msDuration));
    }
    isRecording() {
        return this._ended === false;
    }
    recordException(exception, time) {
        const attributes = {};
        if (typeof exception === 'string') {
            attributes[ATTR_EXCEPTION_MESSAGE] = exception;
        }
        else if (exception) {
            if (exception.code) {
                attributes[ATTR_EXCEPTION_TYPE] = exception.code.toString();
            }
            else if (exception.name) {
                attributes[ATTR_EXCEPTION_TYPE] = exception.name;
            }
            if (exception.message) {
                attributes[ATTR_EXCEPTION_MESSAGE] = exception.message;
            }
            if (exception.stack) {
                attributes[ATTR_EXCEPTION_STACKTRACE] = exception.stack;
            }
        }
        // these are minimum requirements from spec
        if (attributes[ATTR_EXCEPTION_TYPE] || attributes[ATTR_EXCEPTION_MESSAGE]) {
            this.addEvent(ExceptionEventName, attributes, time);
        }
        else {
            diag_api_diag.warn(`Failed to record an exception ${exception}`);
        }
    }
    get duration() {
        return this._duration;
    }
    get ended() {
        return this._ended;
    }
    get droppedAttributesCount() {
        return this._droppedAttributesCount;
    }
    get droppedEventsCount() {
        return this._droppedEventsCount;
    }
    get droppedLinksCount() {
        return this._droppedLinksCount;
    }
    _isSpanEnded() {
        if (this._ended) {
            const error = new Error(`Operation attempted on ended Span {traceId: ${this._spanContext.traceId}, spanId: ${this._spanContext.spanId}}`);
            diag_api_diag.warn(`Cannot execute the operation on ended Span {traceId: ${this._spanContext.traceId}, spanId: ${this._spanContext.spanId}}`, error);
        }
        return this._ended;
    }
    // Utility function to truncate given value within size
    // for value type of string, will truncate to given limit
    // for type of non-string, will return same value
    _truncateToLimitUtil(value, limit) {
        if (value.length <= limit) {
            return value;
        }
        return value.substring(0, limit);
    }
    /**
     * If the given attribute value is of type string and has more characters than given {@code attributeValueLengthLimit} then
     * return string with truncated to {@code attributeValueLengthLimit} characters
     *
     * If the given attribute value is array of strings then
     * return new array of strings with each element truncated to {@code attributeValueLengthLimit} characters
     *
     * Otherwise return same Attribute {@code value}
     *
     * @param value Attribute value
     * @returns truncated attribute value if required, otherwise same value
     */
    _truncateToSize(value) {
        const limit = this._attributeValueLengthLimit;
        // Check limit
        if (limit <= 0) {
            // Negative values are invalid, so do not truncate
            diag_api_diag.warn(`Attribute value limit must be positive, got ${limit}`);
            return value;
        }
        // String
        if (typeof value === 'string') {
            return this._truncateToLimitUtil(value, limit);
        }
        // Array of strings
        if (Array.isArray(value)) {
            return value.map(val => typeof val === 'string' ? this._truncateToLimitUtil(val, limit) : val);
        }
        // Other types, no need to apply value length limit
        return value;
    }
    [inspectCustom](depth, options, inspect) {
        const payload = {
            name: this.name,
            kind: this.kind,
            spanContext: this._spanContext,
            parentSpanContext: this.parentSpanContext,
            status: this.status,
            startTime: this.startTime,
            endTime: this.endTime,
            duration: this._duration,
            ended: this._ended,
            attributes: this.attributes,
            events: this.events,
            links: this.links,
            droppedAttributesCount: this._droppedAttributesCount,
            droppedEventsCount: this._droppedEventsCount,
            droppedLinksCount: this._droppedLinksCount,
            instrumentationScope: this.instrumentationScope,
            resource: { attributes: settledResourceAttributes(this.resource) },
        };
        return formatInspect('SpanImpl', payload, depth, options, inspect);
    }
}
//# sourceMappingURL=Span.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+sdk-trace@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/sdk-trace/build/esm/semconv.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
/*
 * This file contains a copy of unstable semantic convention definitions
 * used by this package.
 * @see https://github.com/open-telemetry/opentelemetry-js/tree/main/semantic-conventions#unstable-semconv
 */
/**
 * A name uniquely identifying the instance of the OpenTelemetry component within its containing SDK instance.
 *
 * @example otlp_grpc_span_exporter/0
 * @example custom-name
 *
 * @note Implementations **SHOULD** ensure a low cardinality for this attribute, even across application or SDK restarts.
 * E.g. implementations **MUST NOT** use UUIDs as values for this attribute.
 *
 * Implementations **MAY** achieve these goals by following a `<otel.component.type>/<instance-counter>` pattern, e.g. `batching_span_processor/0`.
 * Hereby `otel.component.type` refers to the corresponding attribute value of the component.
 *
 * The value of `instance-counter` **MAY** be automatically assigned by the component and uniqueness within the enclosing SDK instance **MUST** be guaranteed.
 * For example, `<instance-counter>` **MAY** be implemented by using a monotonically increasing counter (starting with `0`), which is incremented every time an
 * instance of the given component type is started.
 *
 * With this implementation, for example the first Batching Span Processor would have `batching_span_processor/0`
 * as `otel.component.name`, the second one `batching_span_processor/1` and so on.
 * These values will therefore be reused in the case of an application restart.
 *
 * @experimental This attribute is experimental and is subject to breaking changes in minor releases of `@opentelemetry/semantic-conventions`.
 */
const semconv_ATTR_OTEL_COMPONENT_NAME = 'otel.component.name';
/**
 * A name identifying the type of the OpenTelemetry component.
 *
 * @example batching_span_processor
 * @example com.example.MySpanExporter
 *
 * @note If none of the standardized values apply, implementations **SHOULD** use the language-defined name of the type.
 * E.g. for Java the fully qualified classname **SHOULD** be used in this case.
 *
 * @experimental This attribute is experimental and is subject to breaking changes in minor releases of `@opentelemetry/semantic-conventions`.
 */
const semconv_ATTR_OTEL_COMPONENT_TYPE = 'otel.component.type';
/**
 * Determines whether the span has a parent span, and if so, [whether it is a remote parent](https://opentelemetry.io/docs/specs/otel/trace/api/#isremote)
 *
 * @experimental This attribute is experimental and is subject to breaking changes in minor releases of `@opentelemetry/semantic-conventions`.
 */
const ATTR_OTEL_SPAN_PARENT_ORIGIN = 'otel.span.parent.origin';
/**
 * The result value of the sampler for this span
 *
 * @experimental This attribute is experimental and is subject to breaking changes in minor releases of `@opentelemetry/semantic-conventions`.
 */
const ATTR_OTEL_SPAN_SAMPLING_RESULT = 'otel.span.sampling_result';
/**
 * The number of spans for which the processing has finished, either successful or failed.
 *
 * @note For successful processing, `error.type` **MUST NOT** be set. For failed processing, `error.type` **MUST** contain the failure cause.
 * For the SDK Simple and Batching Span Processor a span is considered to be processed already when it has been submitted to the exporter, not when the corresponding export call has finished.
 *
 * @experimental This metric is experimental and is subject to breaking changes in minor releases of `@opentelemetry/semantic-conventions`.
 */
const METRIC_OTEL_SDK_PROCESSOR_SPAN_PROCESSED = 'otel.sdk.processor.span.processed';
/**
 * The maximum number of spans the queue of a given instance of an SDK span processor can hold.
 *
 * @note Only applies to span processors which use a queue, e.g. the SDK Batching Span Processor.
 *
 * @experimental This metric is experimental and is subject to breaking changes in minor releases of `@opentelemetry/semantic-conventions`.
 */
const METRIC_OTEL_SDK_PROCESSOR_SPAN_QUEUE_CAPACITY = 'otel.sdk.processor.span.queue.capacity';
/**
 * The number of spans in the queue of a given instance of an SDK span processor.
 *
 * @note Only applies to span processors which use a queue, e.g. the SDK Batching Span Processor.
 *
 * @experimental This metric is experimental and is subject to breaking changes in minor releases of `@opentelemetry/semantic-conventions`.
 */
const METRIC_OTEL_SDK_PROCESSOR_SPAN_QUEUE_SIZE = 'otel.sdk.processor.span.queue.size';
/**
 * The number of created spans with `recording=true` for which the end operation has not been called yet.
 *
 * @experimental This metric is experimental and is subject to breaking changes in minor releases of `@opentelemetry/semantic-conventions`.
 */
const METRIC_OTEL_SDK_SPAN_LIVE = 'otel.sdk.span.live';
/**
 * The number of created spans.
 *
 * @note Implementations **MUST** record this metric for all spans, even for non-recording ones.
 *
 * @experimental This metric is experimental and is subject to breaking changes in minor releases of `@opentelemetry/semantic-conventions`.
 */
const METRIC_OTEL_SDK_SPAN_STARTED = 'otel.sdk.span.started';
/**
 * Enum value "batching_span_processor" for attribute {@link ATTR_OTEL_COMPONENT_TYPE}.
 *
 * The builtin SDK batching span processor
 *
 * @experimental This enum value is experimental and is subject to breaking changes in minor releases of `@opentelemetry/semantic-conventions`.
 */
const OTEL_COMPONENT_TYPE_VALUE_BATCHING_SPAN_PROCESSOR = 'batching_span_processor';
/**
 * Enum value "simple_span_processor" for attribute {@link ATTR_OTEL_COMPONENT_TYPE}.
 *
 * The builtin SDK simple span processor
 *
 * @experimental This enum value is experimental and is subject to breaking changes in minor releases of `@opentelemetry/semantic-conventions`.
 */
const OTEL_COMPONENT_TYPE_VALUE_SIMPLE_SPAN_PROCESSOR = 'simple_span_processor';
//# sourceMappingURL=semconv.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+sdk-trace@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/sdk-trace/build/esm/TracerMetrics.js


/**
 * Generates `otel.sdk.span.*` metrics.
 * https://opentelemetry.io/docs/specs/semconv/otel/sdk-metrics/#span-metrics
 */
class TracerMetrics {
    startedSpans;
    liveSpans;
    constructor(meter) {
        this.startedSpans = meter.createCounter(METRIC_OTEL_SDK_SPAN_STARTED, {
            unit: '{span}',
            description: 'The number of created spans.',
        });
        this.liveSpans = meter.createUpDownCounter(METRIC_OTEL_SDK_SPAN_LIVE, {
            unit: '{span}',
            description: 'The number of currently live spans.',
        });
    }
    startSpan(parentSpanCtx, samplingDecision) {
        const samplingDecisionStr = samplingDecisionToString(samplingDecision);
        this.startedSpans.add(1, {
            [ATTR_OTEL_SPAN_PARENT_ORIGIN]: parentOrigin(parentSpanCtx),
            [ATTR_OTEL_SPAN_SAMPLING_RESULT]: samplingDecisionStr,
        });
        if (samplingDecision === SamplingDecision.NOT_RECORD) {
            return () => { };
        }
        const liveSpanAttributes = {
            [ATTR_OTEL_SPAN_SAMPLING_RESULT]: samplingDecisionStr,
        };
        this.liveSpans.add(1, liveSpanAttributes);
        return () => {
            this.liveSpans.add(-1, liveSpanAttributes);
        };
    }
}
function parentOrigin(parentSpanContext) {
    if (!parentSpanContext) {
        return 'none';
    }
    if (parentSpanContext.isRemote) {
        return 'remote';
    }
    return 'local';
}
function samplingDecisionToString(decision) {
    switch (decision) {
        case SamplingDecision.RECORD_AND_SAMPLED:
            return 'RECORD_AND_SAMPLE';
        case SamplingDecision.RECORD:
            return 'RECORD_ONLY';
        case SamplingDecision.NOT_RECORD:
            return 'DROP';
    }
}
//# sourceMappingURL=TracerMetrics.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+sdk-trace@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/sdk-trace/build/esm/version.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
// this is autogenerated file, see scripts/version-update.js
const sdk_trace_build_esm_version_VERSION = '2.11.0';
//# sourceMappingURL=version.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+sdk-trace@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/sdk-trace/build/esm/Tracer.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */






/**
 * This class represents a basic tracer.
 */
class Tracer {
    _sampler;
    _spanLimits;
    _idGenerator;
    instrumentationScope;
    _resource;
    _spanProcessor;
    _tracerMetrics;
    /**
     * Constructs a new Tracer instance.
     */
    constructor(instrumentationScope, options) {
        this.instrumentationScope = instrumentationScope;
        this._sampler = options.sampler;
        this._spanLimits = options.spanLimits;
        this._resource = options.resource;
        this._idGenerator = options.idGenerator;
        this._spanProcessor = options.spanProcessor;
        const meter = options.meterProvider.getMeter('@opentelemetry/sdk-trace', sdk_trace_build_esm_version_VERSION);
        this._tracerMetrics = new TracerMetrics(meter);
    }
    /**
     * Starts a new Span or returns the default NoopSpan based on the sampling
     * decision.
     */
    startSpan(name, options = {}, context = context_api_context.active()) {
        // remove span from context in case a root span is requested via options
        if (options.root) {
            context = trace_api/* trace */.u.deleteSpan(context);
        }
        const parentSpan = trace_api/* trace */.u.getSpan(context);
        if (isTracingSuppressed(context)) {
            diag_api_diag.debug('Instrumentation suppressed, returning Noop Span');
            const nonRecordingSpan = trace_api/* trace */.u.wrapSpanContext(invalid_span_constants/* INVALID_SPAN_CONTEXT */.dM);
            return nonRecordingSpan;
        }
        const parentSpanContext = parentSpan?.spanContext();
        const spanId = this._idGenerator.generateSpanId();
        let validParentSpanContext;
        let traceId;
        let traceState;
        if (!parentSpanContext ||
            !trace_api/* trace */.u.isSpanContextValid(parentSpanContext)) {
            // New root span.
            traceId = this._idGenerator.generateTraceId();
        }
        else {
            // New child span.
            traceId = parentSpanContext.traceId;
            traceState = parentSpanContext.traceState;
            validParentSpanContext = parentSpanContext;
        }
        const spanKind = options.kind ?? SpanKind.INTERNAL;
        const links = (options.links ?? []).map(link => {
            return {
                context: link.context,
                attributes: sanitizeAttributes(link.attributes),
            };
        });
        const attributes = sanitizeAttributes(options.attributes);
        // make sampling decision
        const samplingResult = this._sampler.shouldSample(context, traceId, name, spanKind, attributes, links);
        const recordEndMetrics = this._tracerMetrics.startSpan(parentSpanContext, samplingResult.decision);
        traceState = samplingResult.traceState ?? traceState;
        const traceFlags = samplingResult.decision === SamplingResult_SamplingDecision.RECORD_AND_SAMPLED
            ? trace_flags/* TraceFlags */.X.SAMPLED
            : trace_flags/* TraceFlags */.X.NONE;
        const spanContext = { traceId, spanId, traceFlags, traceState };
        if (samplingResult.decision === SamplingResult_SamplingDecision.NOT_RECORD) {
            diag_api_diag.debug('Recording is off, propagating context in a non-recording span');
            const nonRecordingSpan = trace_api/* trace */.u.wrapSpanContext(spanContext);
            return nonRecordingSpan;
        }
        // Set initial span attributes. The attributes object may have been mutated
        // by the sampler, so we sanitize the merged attributes before setting them.
        const initAttributes = sanitizeAttributes(Object.assign(attributes, samplingResult.attributes));
        const span = new SpanImpl({
            resource: this._resource,
            scope: this.instrumentationScope,
            context,
            spanContext,
            name,
            kind: spanKind,
            links,
            parentSpanContext: validParentSpanContext,
            attributes: initAttributes,
            startTime: options.startTime,
            spanProcessor: this._spanProcessor,
            spanLimits: this._spanLimits,
            recordEndMetrics,
        });
        return span;
    }
    startActiveSpan(name, arg2, arg3, arg4) {
        let opts;
        let ctx;
        let fn;
        if (arguments.length < 2) {
            return;
        }
        else if (arguments.length === 2) {
            fn = arg2;
        }
        else if (arguments.length === 3) {
            opts = arg2;
            fn = arg3;
        }
        else {
            opts = arg2;
            ctx = arg3;
            fn = arg4;
        }
        const parentContext = ctx ?? context_api_context.active();
        const span = this.startSpan(name, opts, parentContext);
        const contextWithSpanSet = trace_api/* trace */.u.setSpan(parentContext, span);
        return context_api_context.with(contextWithSpanSet, fn, undefined, span);
    }
    [inspectCustom](depth, options, inspect) {
        const payload = {
            instrumentationScope: this.instrumentationScope,
            resource: { attributes: settledResourceAttributes(this._resource) },
            spanLimits: this._spanLimits,
        };
        return formatInspect('Tracer', payload, depth, options, inspect);
    }
}
//# sourceMappingURL=Tracer.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+sdk-trace@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/sdk-trace/build/esm/MultiSpanProcessor.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */

/**
 * Implementation of the {@link SpanProcessor} that simply forwards all
 * received events to a list of {@link SpanProcessor}s.
 */
class MultiSpanProcessor {
    _spanProcessors;
    constructor(spanProcessors) {
        this._spanProcessors = spanProcessors;
    }
    forceFlush() {
        const promises = [];
        for (const spanProcessor of this._spanProcessors) {
            promises.push(spanProcessor.forceFlush());
        }
        return new Promise(resolve => {
            Promise.all(promises)
                .then(() => {
                resolve();
            })
                .catch(error => {
                globalErrorHandler(error || new Error('MultiSpanProcessor: forceFlush failed'));
                resolve();
            });
        });
    }
    onStart(span, context) {
        for (const spanProcessor of this._spanProcessors) {
            spanProcessor.onStart(span, context);
        }
    }
    onEnding(span) {
        for (const spanProcessor of this._spanProcessors) {
            if (spanProcessor.onEnding) {
                spanProcessor.onEnding(span);
            }
        }
    }
    onEnd(span) {
        for (const spanProcessor of this._spanProcessors) {
            spanProcessor.onEnd(span);
        }
    }
    shutdown() {
        const promises = [];
        for (const spanProcessor of this._spanProcessors) {
            promises.push(spanProcessor.shutdown());
        }
        return new Promise((resolve, reject) => {
            Promise.all(promises).then(() => {
                resolve();
            }, reject);
        });
    }
}
//# sourceMappingURL=MultiSpanProcessor.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+sdk-trace@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/sdk-trace/build/esm/platform/node/RandomIdGenerator.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
const SPAN_ID_BYTES = 8;
const TRACE_ID_BYTES = 16;
class RandomIdGenerator {
    /**
     * Returns a random 16-byte trace ID formatted/encoded as a 32 lowercase hex
     * characters corresponding to 128 bits.
     */
    generateTraceId = getIdGenerator(TRACE_ID_BYTES);
    /**
     * Returns a random 8-byte span ID formatted/encoded as a 16 lowercase hex
     * characters corresponding to 64 bits.
     */
    generateSpanId = getIdGenerator(SPAN_ID_BYTES);
}
const SHARED_BUFFER = Buffer.allocUnsafe(TRACE_ID_BYTES);
function getIdGenerator(bytes) {
    return function generateId() {
        for (let i = 0; i < bytes / 4; i++) {
            // unsigned right shift drops decimal part of the number
            // it is required because if a number between 2**32 and 2**32 - 1 is generated, an out of range error is thrown by writeUInt32BE
            SHARED_BUFFER.writeUInt32BE((Math.random() * 2 ** 32) >>> 0, i * 4);
        }
        // If buffer is all 0, set the last byte to 1 to guarantee a valid w3c id is generated
        for (let i = 0; i < bytes; i++) {
            if (SHARED_BUFFER[i] > 0) {
                break;
            }
            else if (i === bytes - 1) {
                SHARED_BUFFER[bytes - 1] = 1;
            }
        }
        return SHARED_BUFFER.toString('hex', 0, bytes);
    };
}
//# sourceMappingURL=RandomIdGenerator.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+sdk-trace@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/sdk-trace/build/esm/TracerProvider.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */








var ForceFlushState;
(function (ForceFlushState) {
    ForceFlushState[ForceFlushState["resolved"] = 0] = "resolved";
    ForceFlushState[ForceFlushState["timeout"] = 1] = "timeout";
    ForceFlushState[ForceFlushState["error"] = 2] = "error";
    ForceFlushState[ForceFlushState["unresolved"] = 3] = "unresolved";
})(ForceFlushState || (ForceFlushState = {}));
/**
 * This class represents a basic tracer provider which platform libraries can extend
 */
class TracerProvider {
    _resource;
    _activeSpanProcessor;
    _forceFlushTimeoutMillis;
    _tracerOptions;
    _tracers = new Map();
    constructor(options = {}) {
        this._forceFlushTimeoutMillis = options.forceFlushTimeoutMillis ?? 30000;
        this._resource = options.resource ?? defaultResource();
        const spanProcessors = options.spanProcessors ?? [];
        this._activeSpanProcessor = new MultiSpanProcessor(spanProcessors);
        this._tracerOptions = {
            resource: this._resource,
            sampler: options.sampler ??
                new ParentBasedSampler({
                    root: new AlwaysOnSampler(),
                }),
            spanLimits: {
                attributeCountLimit: options.spanLimits?.attributeCountLimit ?? 128,
                attributeValueLengthLimit: options.spanLimits?.attributeValueLengthLimit ?? Infinity,
                eventCountLimit: options.spanLimits?.eventCountLimit ?? 128,
                linkCountLimit: options.spanLimits?.linkCountLimit ?? 128,
                attributePerEventCountLimit: options.spanLimits?.attributePerEventCountLimit ?? 128,
                attributePerLinkCountLimit: options.spanLimits?.attributePerLinkCountLimit ?? 128,
            },
            idGenerator: options.idGenerator || new RandomIdGenerator(),
            spanProcessor: this._activeSpanProcessor,
            meterProvider: options.meterProvider ?? {
                getMeter() {
                    return createNoopMeter();
                },
            },
        };
    }
    getTracer(name, version, options) {
        const key = `${name}@${version || ''}:${options?.schemaUrl || ''}`;
        if (!this._tracers.has(key)) {
            this._tracers.set(key, new Tracer({ name, version, schemaUrl: options?.schemaUrl }, this._tracerOptions));
        }
        return this._tracers.get(key);
    }
    forceFlush(options) {
        const timeout = options?.timeoutMillis ?? this._forceFlushTimeoutMillis;
        const promises = this._activeSpanProcessor['_spanProcessors'].map((spanProcessor) => {
            return new Promise(resolve => {
                let state;
                const timeoutInterval = setTimeout(() => {
                    resolve(new Error(`Span processor did not completed within timeout period of ${timeout} ms`));
                    state = ForceFlushState.timeout;
                }, timeout);
                spanProcessor
                    .forceFlush()
                    .then(() => {
                    clearTimeout(timeoutInterval);
                    if (state !== ForceFlushState.timeout) {
                        state = ForceFlushState.resolved;
                        resolve(state);
                    }
                })
                    .catch(error => {
                    clearTimeout(timeoutInterval);
                    state = ForceFlushState.error;
                    resolve(error);
                });
            });
        });
        return new Promise((resolve, reject) => {
            Promise.all(promises)
                .then(results => {
                const errors = results.filter(result => result !== ForceFlushState.resolved);
                if (errors.length > 0) {
                    reject(errors);
                }
                else {
                    resolve();
                }
            })
                .catch(error => reject([error]));
        });
    }
    shutdown() {
        return this._activeSpanProcessor.shutdown();
    }
    [inspectCustom](depth, options, inspect) {
        const processors = this._activeSpanProcessor['_spanProcessors'];
        const payload = {
            resource: { attributes: settledResourceAttributes(this._resource) },
            tracers: Array.from(this._tracers.keys()),
            spanProcessors: processors.map(p => p.constructor?.name ?? 'SpanProcessor'),
        };
        return formatInspect('TracerProvider', payload, depth, options, inspect);
    }
}
//# sourceMappingURL=TracerProvider.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+sdk-trace-base@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/sdk-trace-base/build/esm/BasicTracerProvider-shim.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */




/**
 * A TracerProvider implementation that reads configuration defaults from
 * OTEL_* environment variables per
 * https://opentelemetry.io/docs/specs/otel/configuration/sdk-environment-variables/
 */
class BasicTracerProvider extends TracerProvider {
    constructor(config = {}) {
        const mergedConfig = merge({}, loadDefaultConfig(), reconfigureLimits(config));
        delete mergedConfig.generalLimits;
        super(mergedConfig);
    }
}
//# sourceMappingURL=BasicTracerProvider-shim.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+sdk-trace-web@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/sdk-trace-web/build/esm/StackContextManager.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */

/**
 * Stack Context Manager for managing the state in web
 * it doesn't fully support the async calls though
 */
class StackContextManager {
    /**
     * whether the context manager is enabled or not
     */
    _enabled = false;
    /**
     * Keeps the reference to current context
     */
    _currentContext = context_context/* ROOT_CONTEXT */.l;
    /**
     *
     * @param context
     * @param target Function to be executed within the context
     */
    // eslint-disable-next-line @typescript-eslint/no-unsafe-function-type
    _bindFunction(context = context_context/* ROOT_CONTEXT */.l, target) {
        const manager = this;
        const contextWrapper = function (...args) {
            return manager.with(context, () => target.apply(this, args));
        };
        Object.defineProperty(contextWrapper, 'length', {
            enumerable: false,
            configurable: true,
            writable: false,
            value: target.length,
        });
        return contextWrapper;
    }
    /**
     * Returns the active context
     */
    active() {
        return this._currentContext;
    }
    /**
     * Binds a the certain context or the active one to the target function and then returns the target
     * @param context A context (span) to be bind to target
     * @param target a function or event emitter. When target or one of its callbacks is called,
     *  the provided context will be used as the active context for the duration of the call.
     */
    bind(context, target) {
        // if no specific context to propagate is given, we use the current one
        if (context === undefined) {
            context = this.active();
        }
        if (typeof target === 'function') {
            return this._bindFunction(context, target);
        }
        return target;
    }
    /**
     * Disable the context manager (clears the current context)
     */
    disable() {
        this._currentContext = context_context/* ROOT_CONTEXT */.l;
        this._enabled = false;
        return this;
    }
    /**
     * Enables the context manager and creates a default(root) context
     */
    enable() {
        if (this._enabled) {
            return this;
        }
        this._enabled = true;
        this._currentContext = context_context/* ROOT_CONTEXT */.l;
        return this;
    }
    /**
     * Calls the callback function [fn] with the provided [context]. If [context] is undefined then it will use the window.
     * The context will be set as active
     * @param context
     * @param fn Callback function
     * @param thisArg optional receiver to be used for calling fn
     * @param args optional arguments forwarded to fn
     */
    with(context, fn, thisArg, ...args) {
        const previousContext = this._currentContext;
        this._currentContext = context || context_context/* ROOT_CONTEXT */.l;
        try {
            return fn.call(thisArg, ...args);
        }
        finally {
            this._currentContext = previousContext;
        }
    }
}
//# sourceMappingURL=StackContextManager.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+core@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/core/build/esm/propagation/composite.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */

/** Combines multiple propagators into a single propagator. */
class CompositePropagator {
    _propagators;
    _fields;
    /**
     * Construct a composite propagator from a list of propagators.
     *
     * @param [config] Configuration object for composite propagator
     */
    constructor(config = {}) {
        this._propagators = config.propagators ?? [];
        const fields = new Set();
        for (const propagator of this._propagators) {
            // older propagators may not have fields function, null check to be sure
            const propagatorFields = typeof propagator.fields === 'function' ? propagator.fields() : [];
            for (const field of propagatorFields) {
                fields.add(field);
            }
        }
        this._fields = Array.from(fields);
    }
    /**
     * Run each of the configured propagators with the given context and carrier.
     * Propagators are run in the order they are configured, so if multiple
     * propagators write the same carrier key, the propagator later in the list
     * will "win".
     *
     * @param context Context to inject
     * @param carrier Carrier into which context will be injected
     */
    inject(context, carrier, setter) {
        for (const propagator of this._propagators) {
            try {
                propagator.inject(context, carrier, setter);
            }
            catch (err) {
                diag_api_diag.warn(`Failed to inject with ${propagator.constructor.name}. Err: ${err.message}`);
            }
        }
    }
    /**
     * Run each of the configured propagators with the given context and carrier.
     * Propagators are run in the order they are configured, so if multiple
     * propagators write the same context key, the propagator later in the list
     * will "win".
     *
     * @param context Context to add values to
     * @param carrier Carrier from which to extract context
     */
    extract(context, carrier, getter) {
        return this._propagators.reduce((ctx, propagator) => {
            try {
                return propagator.extract(ctx, carrier, getter);
            }
            catch (err) {
                diag_api_diag.warn(`Failed to extract with ${propagator.constructor.name}. Err: ${err.message}`);
            }
            return ctx;
        }, context);
    }
    fields() {
        // return a new array so our fields cannot be modified
        return this._fields.slice();
    }
}
//# sourceMappingURL=composite.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+core@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/core/build/esm/baggage/propagation/W3CBaggagePropagator.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */




/**
 * Propagates {@link Baggage} through Context format propagation.
 *
 * Based on the Baggage specification:
 * https://w3c.github.io/baggage/
 */
class W3CBaggagePropagator {
    inject(context, carrier, setter) {
        const baggage = propagation.getBaggage(context);
        if (!baggage || isTracingSuppressed(context))
            return;
        const keyPairs = getKeyPairs(baggage)
            .filter((pair) => {
            return pair.length <= BAGGAGE_MAX_PER_NAME_VALUE_PAIRS;
        })
            .slice(0, BAGGAGE_MAX_NAME_VALUE_PAIRS);
        const headerValue = serializeKeyPairs(keyPairs);
        if (headerValue.length > 0) {
            setter.set(carrier, BAGGAGE_HEADER, headerValue);
        }
    }
    extract(context, carrier, getter) {
        const headerValue = getter.get(carrier, BAGGAGE_HEADER);
        if (!headerValue) {
            return context;
        }
        const baggage = {};
        let count = 0;
        let totalSize = 0;
        if (Array.isArray(headerValue)) {
            for (let i = 0; i < headerValue.length; i++) {
                [count, totalSize] = parseBaggageHeaderString(headerValue[i], baggage, count, totalSize);
            }
        }
        else {
            [count] = parseBaggageHeaderString(headerValue, baggage, count, totalSize);
        }
        if (count === 0) {
            return context;
        }
        return propagation.setBaggage(context, propagation.createBaggage(baggage));
    }
    fields() {
        return [BAGGAGE_HEADER];
    }
}
//# sourceMappingURL=W3CBaggagePropagator.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+sdk-trace-web@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/sdk-trace-web/build/esm/WebTracerProvider.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */




function setupContextManager(contextManager) {
    // null means 'do not register'
    if (contextManager === null) {
        return;
    }
    // undefined means 'register default'
    if (contextManager === undefined) {
        const defaultContextManager = new StackContextManager();
        defaultContextManager.enable();
        context_api_context.setGlobalContextManager(defaultContextManager);
        return;
    }
    contextManager.enable();
    context_api_context.setGlobalContextManager(contextManager);
}
function setupPropagator(propagator) {
    // null means 'do not register'
    if (propagator === null) {
        return;
    }
    // undefined means 'register default'
    if (propagator === undefined) {
        propagation.setGlobalPropagator(new CompositePropagator({
            propagators: [
                new W3CTraceContextPropagator(),
                new W3CBaggagePropagator(),
            ],
        }));
        return;
    }
    propagation.setGlobalPropagator(propagator);
}
/**
 * This class represents a web tracer with {@link StackContextManager}
 */
class WebTracerProvider extends BasicTracerProvider {
    /**
     * Constructs a new Tracer instance.
     * @param config Web Tracer config
     */
    constructor(config = {}) {
        super(config);
    }
    /**
     * Register this TracerProvider for use with the OpenTelemetry API.
     * Undefined values may be replaced with defaults, and
     * null values will be skipped.
     *
     * @param config Configuration object for SDK registration
     */
    register(config = {}) {
        trace_api/* trace */.u.setGlobalTracerProvider(this);
        setupPropagator(config.propagator);
        setupContextManager(config.contextManager);
    }
}
//# sourceMappingURL=WebTracerProvider.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+core@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/core/build/esm/utils/promise.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
class Deferred {
    _promise;
    _resolve;
    _reject;
    constructor() {
        this._promise = new Promise((resolve, reject) => {
            this._resolve = resolve;
            this._reject = reject;
        });
    }
    get promise() {
        return this._promise;
    }
    resolve(val) {
        this._resolve(val);
    }
    reject(err) {
        this._reject(err);
    }
}
//# sourceMappingURL=promise.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+core@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/core/build/esm/utils/callback.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */

/**
 * Bind the callback and only invoke the callback once regardless how many times `BindOnceFuture.call` is invoked.
 */
class BindOnceFuture {
    _isCalled = false;
    _deferred = new Deferred();
    _callback;
    _that;
    constructor(callback, that) {
        this._callback = callback;
        this._that = that;
    }
    get isCalled() {
        return this._isCalled;
    }
    get promise() {
        return this._deferred.promise;
    }
    call(...args) {
        if (!this._isCalled) {
            this._isCalled = true;
            try {
                Promise.resolve(this._callback.call(this._that, ...args)).then(val => this._deferred.resolve(val), err => this._deferred.reject(err));
            }
            catch (err) {
                this._deferred.reject(err);
            }
        }
        return this._deferred.promise;
    }
}
//# sourceMappingURL=callback.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+sdk-trace@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/sdk-trace/build/esm/export/SpanProcessorMetrics.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */


const SpanProcessorMetrics_componentCounter = new Map();
class SpanProcessorMetrics {
    processedSpans;
    queueSize;
    queueSizeCallback;
    standardAttrs;
    droppedAttrs;
    constructor(componentType, meter, queueConfig) {
        const counter = SpanProcessorMetrics_componentCounter.get(componentType) ?? 0;
        SpanProcessorMetrics_componentCounter.set(componentType, counter + 1);
        this.standardAttrs = {
            [semconv_ATTR_OTEL_COMPONENT_TYPE]: componentType,
            [semconv_ATTR_OTEL_COMPONENT_NAME]: `${componentType}/${counter}`,
        };
        this.droppedAttrs = {
            ...this.standardAttrs,
            [ATTR_ERROR_TYPE]: 'queue_full',
        };
        this.processedSpans = meter.createCounter(METRIC_OTEL_SDK_PROCESSOR_SPAN_PROCESSED, {
            unit: '{span}',
            description: 'The number of spans for which the processing has finished, either successful or failed.',
        });
        if (queueConfig) {
            const { capacity, getQueueSize } = queueConfig;
            const queueCapacity = meter.createUpDownCounter(METRIC_OTEL_SDK_PROCESSOR_SPAN_QUEUE_CAPACITY, {
                unit: '{span}',
                description: 'The maximum number of spans the queue of a given instance of an SDK span processor can hold.',
            });
            queueCapacity.add(capacity, this.standardAttrs);
            this.queueSize = meter.createObservableUpDownCounter(METRIC_OTEL_SDK_PROCESSOR_SPAN_QUEUE_SIZE, {
                unit: '{span}',
                description: 'The number of spans in the queue of a given instance of an SDK span processor.',
            });
            this.queueSizeCallback = result => result.observe(getQueueSize(), this.standardAttrs);
            this.queueSize.addCallback(this.queueSizeCallback);
        }
    }
    dropSpans(count) {
        this.processedSpans.add(count, this.droppedAttrs);
    }
    finishSpans(count, error) {
        if (!error) {
            this.processedSpans.add(count, this.standardAttrs);
            return;
        }
        const attrs = {
            ...this.standardAttrs,
            [ATTR_ERROR_TYPE]: error.name,
        };
        this.processedSpans.add(count, attrs);
    }
    shutdown() {
        if (this.queueSize && this.queueSizeCallback) {
            this.queueSize.removeCallback(this.queueSizeCallback);
        }
    }
}
//# sourceMappingURL=SpanProcessorMetrics.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+sdk-trace@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/sdk-trace/build/esm/export/BatchSpanProcessorBase.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */




/**
 * Implementation of the {@link SpanProcessor} that batches spans exported by
 * the SDK then pushes them to the exporter pipeline.
 */
class BatchSpanProcessorBase {
    _maxExportBatchSize;
    _maxQueueSize;
    _scheduledDelayMillis;
    _exportTimeoutMillis;
    _exporter;
    _metrics;
    _isExporting = false;
    _finishedSpans = [];
    _timer;
    _shutdownOnce;
    _droppedSpansCount = 0;
    constructor(options) {
        this._exporter = options.exporter;
        this._maxExportBatchSize = options.maxExportBatchSize ?? 512;
        this._maxQueueSize = options.maxQueueSize ?? 2048;
        this._scheduledDelayMillis = options.scheduledDelayMillis ?? 5000;
        this._exportTimeoutMillis = options.exportTimeoutMillis ?? 30000;
        this._shutdownOnce = new BindOnceFuture(this._shutdown, this);
        if (this._maxExportBatchSize > this._maxQueueSize) {
            diag_api_diag.warn('BatchSpanProcessor: maxExportBatchSize must be smaller or equal to maxQueueSize, setting maxExportBatchSize to match maxQueueSize');
            this._maxExportBatchSize = this._maxQueueSize;
        }
        const meter = options.selfObsMeterProvider
            ? options.selfObsMeterProvider.getMeter('@opentelemetry/sdk-trace')
            : createNoopMeter();
        this._metrics = new SpanProcessorMetrics(OTEL_COMPONENT_TYPE_VALUE_BATCHING_SPAN_PROCESSOR, meter, {
            capacity: this._maxQueueSize,
            getQueueSize: () => this._finishedSpans.length,
        });
    }
    forceFlush() {
        if (this._shutdownOnce.isCalled) {
            return this._shutdownOnce.promise;
        }
        return this._flushAll();
    }
    // does nothing.
    onStart(_span, _parentContext) { }
    onEnd(span) {
        if (this._shutdownOnce.isCalled) {
            return;
        }
        if ((span.spanContext().traceFlags & trace_flags/* TraceFlags */.X.SAMPLED) === 0) {
            return;
        }
        this._addToBuffer(span);
    }
    shutdown() {
        return this._shutdownOnce.call();
    }
    _shutdown() {
        return Promise.resolve()
            .then(() => {
            return this.onShutdown();
        })
            .then(() => {
            return this._flushAll();
        })
            .then(() => {
            this._metrics.shutdown();
            return this._exporter.shutdown();
        });
    }
    /** Add a span in the buffer. */
    _addToBuffer(span) {
        if (this._finishedSpans.length >= this._maxQueueSize) {
            // limit reached, drop span
            if (this._droppedSpansCount === 0) {
                diag_api_diag.debug('maxQueueSize reached, dropping spans');
            }
            this._droppedSpansCount++;
            this._metrics.dropSpans(1);
            return;
        }
        if (this._droppedSpansCount > 0) {
            // some spans were dropped, log once with count of spans dropped
            diag_api_diag.warn(`Dropped ${this._droppedSpansCount} spans because maxQueueSize reached`);
            this._droppedSpansCount = 0;
        }
        this._finishedSpans.push(span);
        this._maybeStartTimer();
    }
    /**
     * Send all spans to the exporter respecting the batch size limit
     * This function is used only on forceFlush or shutdown,
     * for all other cases _flush should be used
     * */
    _flushAll() {
        return new Promise((resolve, reject) => {
            const promises = [];
            // calculate number of batches
            const count = Math.ceil(this._finishedSpans.length / this._maxExportBatchSize);
            for (let i = 0, j = count; i < j; i++) {
                promises.push(this._flushOneBatch());
            }
            Promise.all(promises)
                .then(() => {
                resolve();
            })
                .catch(reject);
        });
    }
    _flushOneBatch() {
        this._clearTimer();
        if (this._finishedSpans.length === 0) {
            return Promise.resolve();
        }
        return new Promise((resolve, reject) => {
            const timer = setTimeout(() => {
                // don't wait anymore for export, this way the next batch can start
                reject(new Error('Timeout'));
            }, this._exportTimeoutMillis);
            // prevent downstream exporter calls from generating spans
            context_api_context.with(suppressTracing(context_api_context.active()), () => {
                // Reset the finished spans buffer here because the next invocations of the _flush method
                // could pass the same finished spans to the exporter if the buffer is cleared
                // outside the execution of this callback.
                let spans;
                if (this._finishedSpans.length <= this._maxExportBatchSize) {
                    spans = this._finishedSpans;
                    this._finishedSpans = [];
                }
                else {
                    spans = this._finishedSpans.splice(0, this._maxExportBatchSize);
                }
                const doExport = () => this._exporter.export(spans, result => {
                    clearTimeout(timer);
                    this._metrics.finishSpans(spans.length, result.error);
                    if (result.code === ExportResultCode.SUCCESS) {
                        resolve();
                    }
                    else {
                        reject(result.error ??
                            new Error('BatchSpanProcessor: span export failed'));
                    }
                });
                let pendingResources = null;
                for (let i = 0, len = spans.length; i < len; i++) {
                    const span = spans[i];
                    if (span.resource.asyncAttributesPending &&
                        span.resource.waitForAsyncAttributes) {
                        pendingResources ??= [];
                        pendingResources.push(span.resource.waitForAsyncAttributes());
                    }
                }
                // Avoid scheduling a promise to make the behavior more predictable and easier to test
                if (pendingResources === null) {
                    doExport();
                }
                else {
                    Promise.all(pendingResources).then(doExport, err => {
                        globalErrorHandler(err);
                        reject(err);
                    });
                }
            });
        });
    }
    _maybeStartTimer() {
        if (this._isExporting)
            return;
        const flush = () => {
            this._isExporting = true;
            this._flushOneBatch()
                .finally(() => {
                this._isExporting = false;
                if (this._finishedSpans.length > 0) {
                    this._clearTimer();
                    this._maybeStartTimer();
                }
            })
                .catch(e => {
                this._isExporting = false;
                globalErrorHandler(e);
            });
        };
        // we only wait if the queue doesn't have enough elements yet
        if (this._finishedSpans.length >= this._maxExportBatchSize) {
            return flush();
        }
        if (this._timer !== undefined)
            return;
        this._timer = setTimeout(() => flush(), this._scheduledDelayMillis);
        // depending on runtime, this may be a 'number' or NodeJS.Timeout
        if (typeof this._timer !== 'number') {
            this._timer.unref();
        }
    }
    _clearTimer() {
        if (this._timer !== undefined) {
            clearTimeout(this._timer);
            this._timer = undefined;
        }
    }
}
//# sourceMappingURL=BatchSpanProcessorBase.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+sdk-trace@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/sdk-trace/build/esm/platform/node/export/BatchSpanProcessor.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */

class BatchSpanProcessor_BatchSpanProcessor extends BatchSpanProcessorBase {
    onShutdown() { }
}
//# sourceMappingURL=BatchSpanProcessor.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+sdk-trace-base@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/sdk-trace-base/build/esm/BatchSpanProcessor-shim.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */


/**
 * A BatchSpanProcessor that applies `OTEL_*` environment variable fallbacks per
 * https://opentelemetry.io/docs/specs/otel/configuration/sdk-environment-variables/
 */
class BatchSpanProcessor extends BatchSpanProcessor_BatchSpanProcessor {
    constructor(exporter, config) {
        if (!config) {
            config = {};
        }
        const envFallbacks = [
            ['maxExportBatchSize', 'OTEL_BSP_MAX_EXPORT_BATCH_SIZE'],
            ['maxQueueSize', 'OTEL_BSP_MAX_QUEUE_SIZE'],
            ['scheduledDelayMillis', 'OTEL_BSP_SCHEDULE_DELAY'],
            ['exportTimeoutMillis', 'OTEL_BSP_EXPORT_TIMEOUT'],
        ];
        for (const [configName, envName] of envFallbacks) {
            if (config[configName] === undefined) {
                const envFallback = getNumberFromEnv(envName);
                if (envFallback !== undefined) {
                    config[configName] = envFallback;
                }
            }
        }
        super({ exporter, ...config });
    }
}
//# sourceMappingURL=BatchSpanProcessor-shim.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+context-zone-peer-dep@2.11.0+0209432b06d5e1ac/node_modules/@opentelemetry/context-zone-peer-dep/build/esm/util.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
/**
 * check if an object has `addEventListener` and `removeEventListener` functions.
 * Generally only called with a `TargetWithEvents` but may be called with an `unknown` value.
 * @param obj - The object to check.
 */
function isListenerObject(obj) {
    return (typeof obj === 'object' &&
        obj !== null &&
        'addEventListener' in obj &&
        typeof obj.addEventListener === 'function' &&
        'removeEventListener' in obj &&
        typeof obj.removeEventListener === 'function');
}
//# sourceMappingURL=util.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+context-zone-peer-dep@2.11.0+0209432b06d5e1ac/node_modules/@opentelemetry/context-zone-peer-dep/build/esm/ZoneContextManager.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */


/* Key name to be used to save a context reference in Zone */
const ZONE_CONTEXT_KEY = 'OT_ZONE_CONTEXT';
/**
 * ZoneContextManager
 * This module provides an easy functionality for tracing action between asynchronous operations in web.
 * It was not possible with standard [StackContextManager]{@link https://github.com/open-telemetry/opentelemetry-js/blob/main/packages/opentelemetry-sdk-trace-web/src/StackContextManager.ts}.
 * It heavily depends on [zone.js]{@link https://www.npmjs.com/package/zone.js}.
 * It stores the information about context in zone. Each Context will have always new Zone;
 * It also supports binding a certain Span to a target that has "addEventListener" and "removeEventListener".
 * When this happens a new zone is being created and the provided Span is being assigned to this zone.
 */
class ZoneContextManager {
    /**
     * whether the context manager is enabled or not
     */
    _enabled = false;
    /**
     * @param context A context (span) to be executed within target function
     * @param target Function to be executed within the context
     */
    // eslint-disable-next-line @typescript-eslint/no-unsafe-function-type
    _bindFunction(context, target) {
        const manager = this;
        const contextWrapper = function (...args) {
            return manager.with(context, () => target.apply(this, args));
        };
        Object.defineProperty(contextWrapper, 'length', {
            enumerable: false,
            configurable: true,
            writable: false,
            value: target.length,
        });
        return contextWrapper;
    }
    /**
     * @param context A context (span) to be bind to target
     * @param obj target object on which the listeners will be patched
     */
    _bindListener(context, obj) {
        const target = obj;
        if (target.__ot_listeners !== undefined) {
            return obj;
        }
        target.__ot_listeners = {};
        if (typeof target.addEventListener === 'function') {
            target.addEventListener = this._patchAddEventListener(target, target.addEventListener, context);
        }
        if (typeof target.removeEventListener === 'function') {
            target.removeEventListener = this._patchRemoveEventListener(target, target.removeEventListener);
        }
        return obj;
    }
    /**
     * Creates a new zone
     * @param zoneName zone name
     * @param context A context (span) to be bind with Zone
     */
    _createZone(zoneName, context) {
        return Zone.current.fork({
            name: zoneName,
            properties: {
                [ZONE_CONTEXT_KEY]: context,
            },
            onCancelTask(parentZoneDelegate, currentZone, targetZone, task) {
                if (task.state === 'notScheduled' || task.state === 'running') {
                    return task;
                }
                return parentZoneDelegate.cancelTask(targetZone, task);
            },
        });
    }
    /**
     * Patches addEventListener method
     * @param target any target that has "addEventListener" method
     * @param original reference to the patched method
     * @param [context] context to be bind to the listener
     */
    _patchAddEventListener(target, original, context) {
        const contextManager = this;
        return function (event, listener, opts) {
            if (target.__ot_listeners === undefined) {
                target.__ot_listeners = {};
            }
            let listeners = target.__ot_listeners[event];
            if (listeners === undefined) {
                listeners = new WeakMap();
                target.__ot_listeners[event] = listeners;
            }
            const patchedListener = contextManager.bind(context, listener);
            // store a weak reference of the user listener to ours
            listeners.set(listener, patchedListener);
            return original.call(this, event, patchedListener, opts);
        };
    }
    /**
     * Patches removeEventListener method
     * @param target any target that has "removeEventListener" method
     * @param original reference to the patched method
     */
    _patchRemoveEventListener(target, original) {
        return function (event, listener) {
            if (target.__ot_listeners === undefined ||
                target.__ot_listeners[event] === undefined) {
                return original.call(this, event, listener);
            }
            const events = target.__ot_listeners[event];
            const patchedListener = events.get(listener);
            events.delete(listener);
            return original.call(this, event, patchedListener || listener);
        };
    }
    /**
     * Returns the active context
     */
    active() {
        if (!this._enabled || !Zone.current) {
            return context_context/* ROOT_CONTEXT */.l;
        }
        return Zone.current.get(ZONE_CONTEXT_KEY) || context_context/* ROOT_CONTEXT */.l;
    }
    /**
     * Binds a the certain context or the active one to the target function and then returns the target
     * @param context A context (span) to be bind to target
     * @param target a function or event emitter. When target or one of its callbacks is called,
     *  the provided context will be used as the active context for the duration of the call.
     */
    bind(context, target) {
        if (typeof target === 'function') {
            return this._bindFunction(context, target);
        }
        else if (isListenerObject(target)) {
            this._bindListener(context, target);
        }
        return target;
    }
    /**
     * Disable the context manager (clears all the contexts)
     */
    disable() {
        this._enabled = false;
        return this;
    }
    /**
     * Enables the context manager and creates a default(root) context
     */
    enable() {
        this._enabled = true;
        return this;
    }
    /**
     * Calls the callback function [fn] with the provided [context].
     *     If [context] is undefined then it will use the active context.
     *     The context will be set as active
     * @param context A context (span) to be called with provided callback
     * @param fn Callback function
     * @param thisArg optional receiver to be used for calling fn
     * @param args optional arguments forwarded to fn
     */
    with(context, fn, thisArg, ...args) {
        let zoneName = 'otel:with';
        if (fn.name) {
            zoneName += `:${fn.name}`;
        }
        return this._createZone(zoneName, context).run(fn, thisArg, args);
    }
}
//# sourceMappingURL=ZoneContextManager.js.map
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/zone.js@0.16.3/node_modules/zone.js/fesm2015/zone.js
var zone = __webpack_require__(53465);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+context-zone@2.11.0+e40b0dfdd726a224/node_modules/@opentelemetry/context-zone/build/esm/index.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */


//# sourceMappingURL=index.js.map
// EXTERNAL MODULE: ./src/lib/telemetry/active-session.ts
var active_session = __webpack_require__(94007);
// EXTERNAL MODULE: ./src/lib/telemetry/span-buffer.ts
var span_buffer = __webpack_require__(88259);
;// ./src/lib/telemetry/otel-init.ts










// Same-origin by default: the Go server proxies this to the real collector
// (server/handlers/otel_proxy_handler.go) so the browser never needs CORS
// access to the collector itself.
const tracesUrl = process.env.NEXT_PUBLIC_OTEL_EXPORTER_OTLP_TRACES_ENDPOINT ?? "/api/otel/v1/traces";
// Writes `session.uuid` onto every span started while a session is focused.
// State lives in active-session.ts (no OTel imports) so components can read
// it without triggering this file's module-level SDK initialisation.
class SessionSpanProcessor {
    onStart(span) {
        const uuid = (0,active_session/* getActiveSessionUUID */.E)();
        if (uuid !== null) {
            span.setAttribute("session.uuid", uuid);
        }
    }
    onEnd(span) {
        if (span.attributes["session.uuid"] === undefined) return;
        (0,span_buffer/* recordSpan */.o)({
            name: span.name,
            traceId: span.spanContext().traceId,
            spanId: span.spanContext().spanId,
            parentSpanId: span.parentSpanContext?.spanId,
            startTimeMs: span.startTime[0] * 1000 + span.startTime[1] / 1e6,
            durationMs: span.duration[0] * 1000 + span.duration[1] / 1e6,
            status: span.status.code,
            attributes: {
                ...span.attributes
            }
        });
    }
    shutdown() {
        return Promise.resolve();
    }
    forceFlush() {
        return Promise.resolve();
    }
}
const provider = new WebTracerProvider({
    resource: resourceFromAttributes({
        [ATTR_SERVICE_NAME]: "stapler-squad-web"
    }),
    spanProcessors: [
        new BatchSpanProcessor(new OTLPTraceExporter({
            url: tracesUrl
        })),
        new SessionSpanProcessor()
    ]
});
provider.register({
    contextManager: new ZoneContextManager()
});
registerInstrumentations({
    instrumentations: [
        new DocumentLoadInstrumentation(),
        new FetchInstrumentation({
            // Don't trace the exporter's own export calls — every export would
            // otherwise emit a span that needs exporting, spamming the trace with
            // noise about itself.
            ignoreUrls: [
                new RegExp(tracesUrl.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"))
            ]
        })
    ]
});


/***/ }),

/***/ 70290:
/***/ ((module, __unused_webpack_exports, __webpack_require__) => {

"use strict";


var sep = (__webpack_require__(33873).sep)

module.exports = function (file) {
  var segments = file.split(sep)
  var index = segments.lastIndexOf('node_modules')

  if (index === -1) return
  if (!segments[index + 1]) return

  var scoped = segments[index + 1][0] === '@'
  var name = scoped ? segments[index + 1] + '/' + segments[index + 2] : segments[index + 1]
  var offset = scoped ? 3 : 2

  var basedir = ''
  var lastBaseDirSegmentIndex = index + offset - 1
  for (var i = 0; i <= lastBaseDirSegmentIndex; i++) {
    if (i === lastBaseDirSegmentIndex) {
      basedir += segments[i]
    } else {
      basedir += segments[i] + sep
    }
  }

  var path = ''
  var lastSegmentIndex = segments.length - 1
  for (var i2 = index + offset; i2 <= lastSegmentIndex; i2++) {
    if (i2 === lastSegmentIndex) {
      path += segments[i2]
    } else {
      path += segments[i2] + sep
    }
  }

  return {
    name: name,
    basedir: basedir,
    path: path
  }
}


/***/ }),

/***/ 78159:
/***/ ((module, __unused_webpack_exports, __webpack_require__) => {

"use strict";


const path = __webpack_require__(33873)
const Module = __webpack_require__(8086)
const debug = __webpack_require__(40900)('require-in-the-middle')
const moduleDetailsFromPath = __webpack_require__(70290)

// Using the default export is discouraged, but kept for backward compatibility.
// Use this instead:
//    const { Hook } = require('require-in-the-middle')
module.exports = Hook
module.exports.Hook = Hook

let builtinModules // Set<string>

/**
 * Is the given module a "core" module?
 * https://nodejs.org/api/modules.html#core-modules
 *
 * @type {(moduleName: string) => boolean}
 */
let isCore
if (Module.isBuiltin) { // Added in node v18.6.0, v16.17.0
  isCore = Module.isBuiltin
} else if (Module.builtinModules) { // Added in node v9.3.0, v8.10.0, v6.13.0
  isCore = moduleName => {
    if (moduleName.startsWith('node:')) {
      return true
    }

    if (builtinModules === undefined) {
      builtinModules = new Set(Module.builtinModules)
    }

    return builtinModules.has(moduleName)
  }
} else {
  throw new Error('\'require-in-the-middle\' requires Node.js >=v9.3.0 or >=v8.10.0')
}

// 'foo/bar.js' or 'foo/bar/index.js' => 'foo/bar'
const normalize = /([/\\]index)?(\.js)?$/

// Cache `onrequire`-patched exports for modules.
//
// Exports for built-in (a.k.a. "core") modules are stored in an internal Map.
//
// Exports for non-core modules are stored on a private field on the `Module`
// object in `require.cache`. This allows users to delete from `require.cache`
// to trigger a re-load (and re-run of the hook's `onrequire`) of a module the
// next time it is required.
// https://nodejs.org/docs/latest/api/all.html#all_modules_requirecache
//
// In some special cases -- e.g. some other `require()` hook swapping out
// `Module._cache` like `@babel/register` -- a non-core module won't be in
// `require.cache`. In that case this falls back to caching on the internal Map.
class ExportsCache {
  constructor () {
    this._localCache = new Map() // <module filename or id> -> <exports>
    this._kRitmExports = Symbol('RitmExports')
  }

  has (filename, isBuiltin) {
    if (this._localCache.has(filename)) {
      return true
    } else if (!isBuiltin) {
      const mod = __webpack_require__.c[filename]
      return !!(mod && this._kRitmExports in mod)
    } else {
      return false
    }
  }

  get (filename, isBuiltin) {
    const cachedExports = this._localCache.get(filename)
    if (cachedExports !== undefined) {
      return cachedExports
    } else if (!isBuiltin) {
      const mod = __webpack_require__.c[filename]
      return (mod && mod[this._kRitmExports])
    }
  }

  set (filename, exports, isBuiltin) {
    if (isBuiltin) {
      this._localCache.set(filename, exports)
    } else if (filename in __webpack_require__.c) {
      __webpack_require__.c[filename][this._kRitmExports] = exports
    } else {
      debug('non-core module is unexpectedly not in require.cache: "%s"', filename)
      this._localCache.set(filename, exports)
    }
  }
}

function Hook (modules, options, onrequire) {
  if ((this instanceof Hook) === false) return new Hook(modules, options, onrequire)
  if (typeof modules === 'function') {
    onrequire = modules
    modules = null
    options = null
  } else if (typeof options === 'function') {
    onrequire = options
    options = null
  }

  if (typeof Module._resolveFilename !== 'function') {
    console.error('Error: Expected Module._resolveFilename to be a function (was: %s) - aborting!', typeof Module._resolveFilename)
    console.error('Please report this error as an issue related to Node.js %s at https://github.com/nodejs/require-in-the-middle/issues', process.version)
    return
  }

  this._cache = new ExportsCache()

  this._unhooked = false
  this._origRequire = Module.prototype.require

  const self = this
  const patching = new Set()
  const internals = options ? options.internals === true : false
  const hasWhitelist = Array.isArray(modules)

  debug('registering require hook')

  this._require = Module.prototype.require = function (id) {
    if (self._unhooked === true) {
      // if the patched require function could not be removed because
      // someone else patched it after it was patched here, we just
      // abort and pass the request onwards to the original require
      debug('ignoring require call - module is soft-unhooked')
      return self._origRequire.apply(this, arguments)
    }

    return patchedRequire.call(this, arguments, false)
  }

  if (typeof process.getBuiltinModule === 'function') {
    this._origGetBuiltinModule = process.getBuiltinModule
    this._getBuiltinModule = process.getBuiltinModule = function (id) {
      if (self._unhooked === true) {
        // if the patched process.getBuiltinModule function could not be removed because
        // someone else patched it after it was patched here, we just abort and pass the
        // request onwards to the original process.getBuiltinModule
        debug('ignoring process.getBuiltinModule call - module is soft-unhooked')
        return self._origGetBuiltinModule.apply(this, arguments)
      }

      return patchedRequire.call(this, arguments, true)
    }
  }

  // Preserve the original require/process.getBuiltinModule arguments in `args`
  function patchedRequire (args, coreOnly) {
    const id = args[0]
    const core = isCore(id)
    let filename // the string used for caching
    if (core) {
      filename = id
      // If this is a builtin module that can be identified both as 'foo' and
      // 'node:foo', then prefer 'foo' as the caching key.
      if (id.startsWith('node:')) {
        const idWithoutPrefix = id.slice(5)
        if (isCore(idWithoutPrefix)) {
          filename = idWithoutPrefix
        }
      }
    } else if (coreOnly) {
      // `coreOnly` is `true` if this was a call to `process.getBuiltinModule`, in which case
      // we don't want to return anything if the requested `id` isn't a core module. Falling
      // back to default behaviour, which at the time of this wrting is simply returning `undefined`
      debug('call to process.getBuiltinModule with unknown built-in id')
      return self._origGetBuiltinModule.apply(this, args)
    } else {
      try {
        filename = Module._resolveFilename(id, this)
      } catch (resolveErr) {
        // If someone *else* monkey-patches before this monkey-patch, then that
        // code might expect `require(someId)` to get through so it can be
        // handled, even if `someId` cannot be resolved to a filename. In this
        // case, instead of throwing we defer to the underlying `require`.
        //
        // For example the Azure Functions Node.js worker module does this,
        // where `@azure/functions-core` resolves to an internal object.
        // https://github.com/Azure/azure-functions-nodejs-worker/blob/v3.5.2/src/setupCoreModule.ts#L46-L54
        debug('Module._resolveFilename("%s") threw %j, calling original Module.require', id, resolveErr.message)
        return self._origRequire.apply(this, args)
      }
    }

    let moduleName, basedir

    debug('processing %s module require(\'%s\'): %s', core === true ? 'core' : 'non-core', id, filename)

    // return known patched modules immediately
    if (self._cache.has(filename, core) === true) {
      debug('returning already patched cached module: %s', filename)
      return self._cache.get(filename, core)
    }

    // Check if this module has a patcher in-progress already.
    // Otherwise, mark this module as patching in-progress.
    const isPatching = patching.has(filename)
    if (isPatching === false) {
      patching.add(filename)
    }

    const exports = coreOnly
      ? self._origGetBuiltinModule.apply(this, args)
      : self._origRequire.apply(this, args)

    // If it's already patched, just return it as-is.
    if (isPatching === true) {
      debug('module is in the process of being patched already - ignoring: %s', filename)
      return exports
    }

    // The module has already been loaded,
    // so the patching mark can be cleaned up.
    patching.delete(filename)

    if (core === true) {
      if (hasWhitelist === true && modules.includes(filename) === false) {
        debug('ignoring core module not on whitelist: %s', filename)
        return exports // abort if module name isn't on whitelist
      }
      moduleName = filename
    } else if (hasWhitelist === true && modules.includes(filename)) {
      // whitelist includes the absolute path to the file including extension
      const parsedPath = path.parse(filename)
      moduleName = parsedPath.name
      basedir = parsedPath.dir
    } else {
      const stat = moduleDetailsFromPath(filename)
      if (stat === undefined) {
        debug('could not parse filename: %s', filename)
        return exports // abort if filename could not be parsed
      }
      moduleName = stat.name
      basedir = stat.basedir

      // Ex: require('foo/lib/../bar.js')
      // moduleName = 'foo'
      // fullModuleName = 'foo/bar'
      const fullModuleName = resolveModuleName(stat)

      debug('resolved filename to module: %s (id: %s, resolved: %s, basedir: %s)', moduleName, id, fullModuleName, basedir)

      let matchFound = false
      if (hasWhitelist) {
        if (!id.startsWith('.') && modules.includes(id)) {
          // Not starting with '.' means `id` is identifying a module path,
          // as opposed to a local file path. (Note: I'm not sure about
          // absolute paths, but those are handled above.)
          // If this `id` is in `modules`, then this could be a match to an
          // package "exports" entry point that wouldn't otherwise match below.
          moduleName = id
          matchFound = true
        }

        // abort if module name isn't on whitelist
        if (!modules.includes(moduleName) && !modules.includes(fullModuleName)) {
          return exports
        }

        if (modules.includes(fullModuleName) && fullModuleName !== moduleName) {
          // if we get to this point, it means that we're requiring a whitelisted sub-module
          moduleName = fullModuleName
          matchFound = true
        }
      }

      if (!matchFound) {
        // figure out if this is the main module file, or a file inside the module
        let res
        try {
          res = __webpack_require__(79277).resolve(moduleName, { paths: [basedir] })
        } catch (e) {
          debug('could not resolve module: %s', moduleName)
          self._cache.set(filename, exports, core)
          return exports // abort if module could not be resolved (e.g. no main in package.json and no index.js file)
        }

        if (res !== filename) {
          // this is a module-internal file
          if (internals === true) {
            // use the module-relative path to the file, prefixed by original module name
            moduleName = moduleName + path.sep + path.relative(basedir, filename)
            debug('preparing to process require of internal file: %s', moduleName)
          } else {
            debug('ignoring require of non-main module file: %s', res)
            self._cache.set(filename, exports, core)
            return exports // abort if not main module file
          }
        }
      }
    }

    // ensure that the cache entry is assigned a value before calling
    // onrequire, in case calling onrequire requires the same module.
    self._cache.set(filename, exports, core)
    debug('calling require hook: %s', moduleName)
    const patchedExports = onrequire(exports, moduleName, basedir)
    self._cache.set(filename, patchedExports, core)

    debug('returning module: %s', moduleName)
    return patchedExports
  }
}

Hook.prototype.unhook = function () {
  this._unhooked = true

  if (this._require === Module.prototype.require) {
    Module.prototype.require = this._origRequire
    debug('require unhook successful')
  } else {
    debug('require unhook unsuccessful')
  }

  if (process.getBuiltinModule !== undefined) {
    if (this._getBuiltinModule === process.getBuiltinModule) {
      process.getBuiltinModule = this._origGetBuiltinModule
      debug('process.getBuiltinModule unhook successful')
    } else {
      debug('process.getBuiltinModule unhook unsuccessful')
    }
  }
}

function resolveModuleName (stat) {
  const normalizedPath = path.sep !== '/' ? stat.path.split(path.sep).join('/') : stat.path
  return path.posix.join(stat.name, normalizedPath).replace(normalize, '')
}


/***/ }),

/***/ 79277:
/***/ ((module) => {

function webpackEmptyContext(req) {
	var e = new Error("Cannot find module '" + req + "'");
	e.code = 'MODULE_NOT_FOUND';
	throw e;
}
webpackEmptyContext.keys = () => ([]);
webpackEmptyContext.resolve = webpackEmptyContext;
webpackEmptyContext.id = 79277;
module.exports = webpackEmptyContext;

/***/ }),

/***/ 88259:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   o: () => (/* binding */ recordSpan),
/* harmony export */   y: () => (/* binding */ getSessionSpansJson)
/* harmony export */ });
// Bounded in-memory record of ended browser spans, read by the diagnostic
// bundle export. No OTel imports so components can use it without triggering
// otel-init.ts's module-level SDK setup.
const MAX_SPANS = 2000;
const spans = [];
function recordSpan(span) {
    spans.push(span);
    if (spans.length > MAX_SPANS) spans.shift();
}
/** Spans tagged with sessionUuid, JSON-encoded for the export request. */ function getSessionSpansJson(sessionUuid) {
    const matching = spans.filter((s)=>s.attributes["session.uuid"] === sessionUuid);
    return new TextEncoder().encode(JSON.stringify(matching));
}


/***/ }),

/***/ 90990:
/***/ ((module, __unused_webpack_exports, __webpack_require__) => {

"use strict";

const os = __webpack_require__(21820);
const tty = __webpack_require__(83997);
const hasFlag = __webpack_require__(60113);

const {env} = process;

let forceColor;
if (hasFlag('no-color') ||
	hasFlag('no-colors') ||
	hasFlag('color=false') ||
	hasFlag('color=never')) {
	forceColor = 0;
} else if (hasFlag('color') ||
	hasFlag('colors') ||
	hasFlag('color=true') ||
	hasFlag('color=always')) {
	forceColor = 1;
}

if ('FORCE_COLOR' in env) {
	if (env.FORCE_COLOR === 'true') {
		forceColor = 1;
	} else if (env.FORCE_COLOR === 'false') {
		forceColor = 0;
	} else {
		forceColor = env.FORCE_COLOR.length === 0 ? 1 : Math.min(parseInt(env.FORCE_COLOR, 10), 3);
	}
}

function translateLevel(level) {
	if (level === 0) {
		return false;
	}

	return {
		level,
		hasBasic: true,
		has256: level >= 2,
		has16m: level >= 3
	};
}

function supportsColor(haveStream, streamIsTTY) {
	if (forceColor === 0) {
		return 0;
	}

	if (hasFlag('color=16m') ||
		hasFlag('color=full') ||
		hasFlag('color=truecolor')) {
		return 3;
	}

	if (hasFlag('color=256')) {
		return 2;
	}

	if (haveStream && !streamIsTTY && forceColor === undefined) {
		return 0;
	}

	const min = forceColor || 0;

	if (env.TERM === 'dumb') {
		return min;
	}

	if (process.platform === 'win32') {
		// Windows 10 build 10586 is the first Windows release that supports 256 colors.
		// Windows 10 build 14931 is the first release that supports 16m/TrueColor.
		const osRelease = os.release().split('.');
		if (
			Number(osRelease[0]) >= 10 &&
			Number(osRelease[2]) >= 10586
		) {
			return Number(osRelease[2]) >= 14931 ? 3 : 2;
		}

		return 1;
	}

	if ('CI' in env) {
		if (['TRAVIS', 'CIRCLECI', 'APPVEYOR', 'GITLAB_CI', 'GITHUB_ACTIONS', 'BUILDKITE'].some(sign => sign in env) || env.CI_NAME === 'codeship') {
			return 1;
		}

		return min;
	}

	if ('TEAMCITY_VERSION' in env) {
		return /^(9\.(0*[1-9]\d*)\.|\d{2,}\.)/.test(env.TEAMCITY_VERSION) ? 1 : 0;
	}

	if (env.COLORTERM === 'truecolor') {
		return 3;
	}

	if ('TERM_PROGRAM' in env) {
		const version = parseInt((env.TERM_PROGRAM_VERSION || '').split('.')[0], 10);

		switch (env.TERM_PROGRAM) {
			case 'iTerm.app':
				return version >= 3 ? 3 : 2;
			case 'Apple_Terminal':
				return 2;
			// No default
		}
	}

	if (/-256(color)?$/i.test(env.TERM)) {
		return 2;
	}

	if (/^screen|^xterm|^vt100|^vt220|^rxvt|color|ansi|cygwin|linux/i.test(env.TERM)) {
		return 1;
	}

	if ('COLORTERM' in env) {
		return 1;
	}

	return min;
}

function getSupportLevel(stream) {
	const level = supportsColor(stream, stream && stream.isTTY);
	return translateLevel(level);
}

module.exports = {
	supportsColor: getSupportLevel,
	stdout: translateLevel(supportsColor(true, tty.isatty(1))),
	stderr: translateLevel(supportsColor(true, tty.isatty(2)))
};


/***/ })

};
;