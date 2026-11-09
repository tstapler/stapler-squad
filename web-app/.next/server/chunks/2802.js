"use strict";
exports.id = 2802;
exports.ids = [2802];
exports.modules = {

/***/ 87936:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  UC: () => (/* binding */ Content2),
  Y9: () => (/* binding */ Header),
  q7: () => (/* binding */ Item),
  bL: () => (/* binding */ Root2),
  l9: () => (/* binding */ Trigger2)
});

// UNUSED EXPORTS: Accordion, AccordionContent, AccordionHeader, AccordionItem, AccordionTrigger, createAccordionScope

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react.js
var react = __webpack_require__(88782);
var react_namespaceObject = /*#__PURE__*/__webpack_require__.t(react, 2);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react-jsx-runtime.js
var react_jsx_runtime = __webpack_require__(79147);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@radix-ui+react-context@1.2.0+e8bcf8c23599d818/node_modules/@radix-ui/react-context/dist/index.mjs
// src/create-context.tsx


function createContext2(rootComponentName, defaultContext) {
  const Context = React.createContext(defaultContext);
  Context.displayName = rootComponentName + "Context";
  const Provider = (props) => {
    const { children, ...context } = props;
    const value = React.useMemo(() => context, Object.values(context));
    return /* @__PURE__ */ jsx(Context.Provider, { value, children });
  };
  Provider.displayName = rootComponentName + "Provider";
  function useContext2(consumerName, options = {}) {
    const { optional = false } = options;
    const context = React.useContext(Context);
    if (context) return context;
    if (defaultContext !== void 0) return defaultContext;
    if (optional) return void 0;
    throw new Error(`\`${consumerName}\` must be used within \`${rootComponentName}\``);
  }
  return [Provider, useContext2];
}
function createContextScope(scopeName, createContextScopeDeps = []) {
  let defaultContexts = [];
  function createContext3(rootComponentName, defaultContext) {
    const BaseContext = react.createContext(defaultContext);
    BaseContext.displayName = rootComponentName + "Context";
    const index = defaultContexts.length;
    defaultContexts = [...defaultContexts, defaultContext];
    const Provider = (props) => {
      const { scope, children, ...context } = props;
      const Context = scope?.[scopeName]?.[index] || BaseContext;
      const value = react.useMemo(() => context, Object.values(context));
      return /* @__PURE__ */ (0,react_jsx_runtime.jsx)(Context.Provider, { value, children });
    };
    Provider.displayName = rootComponentName + "Provider";
    function useContext2(consumerName, scope, options = {}) {
      const { optional = false } = options;
      const Context = scope?.[scopeName]?.[index] || BaseContext;
      const context = react.useContext(Context);
      if (context) return context;
      if (defaultContext !== void 0) return defaultContext;
      if (optional) return void 0;
      throw new Error(`\`${consumerName}\` must be used within \`${rootComponentName}\``);
    }
    return [Provider, useContext2];
  }
  const createScope = () => {
    const scopeContexts = defaultContexts.map((defaultContext) => {
      return react.createContext(defaultContext);
    });
    return function useScope(scope) {
      const contexts = scope?.[scopeName] || scopeContexts;
      return react.useMemo(
        () => ({ [`__scope${scopeName}`]: { ...scope, [scopeName]: contexts } }),
        [scope, contexts]
      );
    };
  };
  createScope.scopeName = scopeName;
  return [createContext3, composeContextScopes(createScope, ...createContextScopeDeps)];
}
function composeContextScopes(...scopes) {
  const baseScope = scopes[0];
  if (scopes.length === 1) return baseScope;
  const createScope = () => {
    const scopeHooks = scopes.map((createScope2) => ({
      useScope: createScope2(),
      scopeName: createScope2.scopeName
    }));
    return function useComposedScopes(overrideScopes) {
      const nextScopes = scopeHooks.reduce((nextScopes2, { useScope, scopeName }) => {
        const scopeProps = useScope(overrideScopes);
        const currentScope = scopeProps[`__scope${scopeName}`];
        return { ...nextScopes2, ...currentScope };
      }, {});
      return react.useMemo(() => ({ [`__scope${baseScope.scopeName}`]: nextScopes }), [nextScopes]);
    };
  };
  createScope.scopeName = baseScope.scopeName;
  return createScope;
}

//# sourceMappingURL=index.mjs.map

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@radix-ui+react-compose-refs@1.1.3+e8bcf8c23599d818/node_modules/@radix-ui/react-compose-refs/dist/index.mjs
// src/compose-refs.tsx

function setRef(ref, value) {
  if (typeof ref === "function") {
    return ref(value);
  } else if (ref !== null && ref !== void 0) {
    ref.current = value;
  }
}
function composeRefs(...refs) {
  return (node) => {
    let hasCleanup = false;
    const cleanups = refs.map((ref) => {
      const cleanup = setRef(ref, node);
      if (!hasCleanup && typeof cleanup == "function") {
        hasCleanup = true;
      }
      return cleanup;
    });
    if (hasCleanup) {
      return () => {
        for (let i = 0; i < cleanups.length; i++) {
          const cleanup = cleanups[i];
          if (typeof cleanup == "function") {
            cleanup();
          } else {
            setRef(refs[i], null);
          }
        }
      };
    }
  };
}
function useComposedRefs(...refs) {
  return react.useCallback(composeRefs(...refs), refs);
}

//# sourceMappingURL=index.mjs.map

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@radix-ui+react-slot@1.3.0+e8bcf8c23599d818/node_modules/@radix-ui/react-slot/dist/index.mjs
// src/slot.tsx


// @__NO_SIDE_EFFECTS__
function createSlot(ownerName) {
  const Slot2 = react.forwardRef((props, forwardedRef) => {
    let { children, ...slotProps } = props;
    let slottableElement = null;
    let hasSlottable = false;
    const newChildren = [];
    if (isLazyComponent(children) && typeof use === "function") {
      children = use(children._payload);
    }
    react.Children.forEach(children, (maybeSlottable) => {
      if (isSlottable(maybeSlottable)) {
        hasSlottable = true;
        const slottable = maybeSlottable;
        let child = "child" in slottable.props ? slottable.props.child : slottable.props.children;
        if (isLazyComponent(child) && typeof use === "function") {
          child = use(child._payload);
        }
        slottableElement = getSlottableElementFromSlottable(slottable, child);
        newChildren.push(slottableElement?.props?.children);
      } else {
        newChildren.push(maybeSlottable);
      }
    });
    if (slottableElement) {
      slottableElement = react.cloneElement(slottableElement, void 0, newChildren);
    } else if (
      // A `Slottable` was found but it didn't resolve to a single element (e.g.
      // it wrapped multiple elements, text, or a render-prop `child` that
      // wasn't an element). Don't fall back to treating the `Slottable` wrapper
      // itself as the slot target — throw a descriptive error below instead.
      !hasSlottable && react.Children.count(children) === 1 && react.isValidElement(children)
    ) {
      slottableElement = children;
    }
    const slottableElementRef = slottableElement ? getElementRef(slottableElement) : void 0;
    const composedRef = useComposedRefs(forwardedRef, slottableElementRef);
    if (!slottableElement) {
      if (children || children === 0) {
        throw new Error(
          hasSlottable ? createSlottableError(ownerName) : createSlotError(ownerName)
        );
      }
      return children;
    }
    const mergedProps = mergeProps(slotProps, slottableElement.props ?? {});
    if (slottableElement.type !== react.Fragment) {
      mergedProps.ref = forwardedRef ? composedRef : slottableElementRef;
    }
    return react.cloneElement(slottableElement, mergedProps);
  });
  Slot2.displayName = `${ownerName}.Slot`;
  return Slot2;
}
var Slot = /* @__PURE__ */ (/* unused pure expression or super */ null && (createSlot("Slot")));
var SLOTTABLE_IDENTIFIER = Symbol.for("radix.slottable");
// @__NO_SIDE_EFFECTS__
function createSlottable(ownerName) {
  const Slottable2 = (props) => "child" in props ? props.children(props.child) : props.children;
  Slottable2.displayName = `${ownerName}.Slottable`;
  Slottable2.__radixId = SLOTTABLE_IDENTIFIER;
  return Slottable2;
}
var Slottable = /* @__PURE__ */ (/* unused pure expression or super */ null && (createSlottable("Slottable")));
var getSlottableElementFromSlottable = (slottable, child) => {
  if ("child" in slottable.props) {
    const child2 = slottable.props.child;
    if (!react.isValidElement(child2)) return null;
    return react.cloneElement(child2, void 0, slottable.props.children(child2.props.children));
  }
  return react.isValidElement(child) ? child : null;
};
function mergeProps(slotProps, childProps) {
  const overrideProps = { ...childProps };
  for (const propName in childProps) {
    const slotPropValue = slotProps[propName];
    const childPropValue = childProps[propName];
    const isHandler = /^on[A-Z]/.test(propName);
    if (isHandler) {
      if (slotPropValue && childPropValue) {
        overrideProps[propName] = (...args) => {
          const result = childPropValue(...args);
          slotPropValue(...args);
          return result;
        };
      } else if (slotPropValue) {
        overrideProps[propName] = slotPropValue;
      }
    } else if (propName === "style") {
      overrideProps[propName] = { ...slotPropValue, ...childPropValue };
    } else if (propName === "className") {
      overrideProps[propName] = [slotPropValue, childPropValue].filter(Boolean).join(" ");
    }
  }
  return { ...slotProps, ...overrideProps };
}
function getElementRef(element) {
  let getter = Object.getOwnPropertyDescriptor(element.props, "ref")?.get;
  let mayWarn = getter && "isReactWarning" in getter && getter.isReactWarning;
  if (mayWarn) {
    return element.ref;
  }
  getter = Object.getOwnPropertyDescriptor(element, "ref")?.get;
  mayWarn = getter && "isReactWarning" in getter && getter.isReactWarning;
  if (mayWarn) {
    return element.props.ref;
  }
  return element.props.ref || element.ref;
}
function isSlottable(child) {
  return react.isValidElement(child) && typeof child.type === "function" && "__radixId" in child.type && child.type.__radixId === SLOTTABLE_IDENTIFIER;
}
var REACT_LAZY_TYPE = Symbol.for("react.lazy");
function isLazyComponent(element) {
  return element != null && typeof element === "object" && "$$typeof" in element && element.$$typeof === REACT_LAZY_TYPE && "_payload" in element && isPromiseLike(element._payload);
}
function isPromiseLike(value) {
  return typeof value === "object" && value !== null && "then" in value;
}
var createSlotError = (ownerName) => {
  return `${ownerName} failed to slot onto its children. Expected a single React element child or \`Slottable\`.`;
};
var createSlottableError = (ownerName) => {
  return `${ownerName} failed to slot onto its \`Slottable\`. Expected \`Slottable\` to receive a single React element child.`;
};
var use = react_namespaceObject[" use ".trim().toString()];

//# sourceMappingURL=index.mjs.map

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@radix-ui+react-collection@1.1.12+b721ee6ccd61acaa/node_modules/@radix-ui/react-collection/dist/index.mjs
/* __next_internal_client_entry_do_not_use__ createCollection,unstable_createCollection auto */ // src/collection-legacy.tsx





function createCollection(name) {
    const PROVIDER_NAME = name + "CollectionProvider";
    const [createCollectionContext, createCollectionScope] = createContextScope(PROVIDER_NAME);
    const [CollectionProviderImpl, useCollectionContext] = createCollectionContext(PROVIDER_NAME, {
        collectionRef: {
            current: null
        },
        itemMap: /* @__PURE__ */ new Map()
    });
    const CollectionProvider = (props)=>{
        const { scope, children } = props;
        const ref = react.useRef(null);
        const itemMap = react.useRef(/* @__PURE__ */ new Map()).current;
        return /* @__PURE__ */ (0,react_jsx_runtime.jsx)(CollectionProviderImpl, {
            scope,
            itemMap,
            collectionRef: ref,
            children
        });
    };
    CollectionProvider.displayName = PROVIDER_NAME;
    const COLLECTION_SLOT_NAME = name + "CollectionSlot";
    const CollectionSlotImpl = createSlot(COLLECTION_SLOT_NAME);
    const CollectionSlot = /*#__PURE__*/ react.forwardRef((props, forwardedRef)=>{
        const { scope, children } = props;
        const context = useCollectionContext(COLLECTION_SLOT_NAME, scope);
        const composedRefs = useComposedRefs(forwardedRef, context.collectionRef);
        return /* @__PURE__ */ (0,react_jsx_runtime.jsx)(CollectionSlotImpl, {
            ref: composedRefs,
            children
        });
    });
    CollectionSlot.displayName = COLLECTION_SLOT_NAME;
    const ITEM_SLOT_NAME = name + "CollectionItemSlot";
    const ITEM_DATA_ATTR = "data-radix-collection-item";
    const CollectionItemSlotImpl = createSlot(ITEM_SLOT_NAME);
    const CollectionItemSlot = /*#__PURE__*/ react.forwardRef((props, forwardedRef)=>{
        const { scope, children, ...itemData } = props;
        const ref = react.useRef(null);
        const composedRefs = useComposedRefs(forwardedRef, ref);
        const context = useCollectionContext(ITEM_SLOT_NAME, scope);
        react.useEffect(()=>{
            context.itemMap.set(ref, {
                ref,
                ...itemData
            });
            return ()=>void context.itemMap.delete(ref);
        });
        return /* @__PURE__ */ (0,react_jsx_runtime.jsx)(CollectionItemSlotImpl, {
            ...{
                [ITEM_DATA_ATTR]: ""
            },
            ref: composedRefs,
            children
        });
    });
    CollectionItemSlot.displayName = ITEM_SLOT_NAME;
    function useCollection(scope) {
        const context = useCollectionContext(name + "CollectionConsumer", scope);
        const getItems = react.useCallback(()=>{
            const collectionNode = context.collectionRef.current;
            if (!collectionNode) return [];
            const orderedNodes = Array.from(collectionNode.querySelectorAll(`[${ITEM_DATA_ATTR}]`));
            const items = Array.from(context.itemMap.values());
            const orderedItems = items.sort((a, b)=>orderedNodes.indexOf(a.ref.current) - orderedNodes.indexOf(b.ref.current));
            return orderedItems;
        }, [
            context.collectionRef,
            context.itemMap
        ]);
        return getItems;
    }
    return [
        {
            Provider: CollectionProvider,
            Slot: CollectionSlot,
            ItemSlot: CollectionItemSlot
        },
        useCollection,
        createCollectionScope
    ];
}
// src/collection.tsx




// src/ordered-dictionary.ts
var __instanciated = /* @__PURE__ */ new WeakMap();
var OrderedDict = class _OrderedDict extends Map {
    #keys;
    constructor(entries){
        super(entries);
        this.#keys = [
            ...super.keys()
        ];
        __instanciated.set(this, true);
    }
    set(key, value) {
        if (__instanciated.get(this)) {
            if (this.has(key)) {
                this.#keys[this.#keys.indexOf(key)] = key;
            } else {
                this.#keys.push(key);
            }
        }
        super.set(key, value);
        return this;
    }
    insert(index, key, value) {
        const has = this.has(key);
        const length = this.#keys.length;
        const relativeIndex = toSafeInteger(index);
        let actualIndex = relativeIndex >= 0 ? relativeIndex : length + relativeIndex;
        const safeIndex = actualIndex < 0 || actualIndex >= length ? -1 : actualIndex;
        if (safeIndex === this.size || has && safeIndex === this.size - 1 || safeIndex === -1) {
            this.set(key, value);
            return this;
        }
        const size = this.size + (has ? 0 : 1);
        if (relativeIndex < 0) {
            actualIndex++;
        }
        const keys = [
            ...this.#keys
        ];
        let nextValue;
        let shouldSkip = false;
        for(let i = actualIndex; i < size; i++){
            if (actualIndex === i) {
                let nextKey = keys[i];
                if (keys[i] === key) {
                    nextKey = keys[i + 1];
                }
                if (has) {
                    this.delete(key);
                }
                nextValue = this.get(nextKey);
                this.set(key, value);
            } else {
                if (!shouldSkip && keys[i - 1] === key) {
                    shouldSkip = true;
                }
                const currentKey = keys[shouldSkip ? i : i - 1];
                const currentValue = nextValue;
                nextValue = this.get(currentKey);
                this.delete(currentKey);
                this.set(currentKey, currentValue);
            }
        }
        return this;
    }
    with(index, key, value) {
        const copy = new _OrderedDict(this);
        copy.insert(index, key, value);
        return copy;
    }
    before(key) {
        const index = this.#keys.indexOf(key) - 1;
        if (index < 0) {
            return void 0;
        }
        return this.entryAt(index);
    }
    /**
   * Sets a new key-value pair at the position before the given key.
   */ setBefore(key, newKey, value) {
        const index = this.#keys.indexOf(key);
        if (index === -1) {
            return this;
        }
        return this.insert(index, newKey, value);
    }
    after(key) {
        let index = this.#keys.indexOf(key);
        index = index === -1 || index === this.size - 1 ? -1 : index + 1;
        if (index === -1) {
            return void 0;
        }
        return this.entryAt(index);
    }
    /**
   * Sets a new key-value pair at the position after the given key.
   */ setAfter(key, newKey, value) {
        const index = this.#keys.indexOf(key);
        if (index === -1) {
            return this;
        }
        return this.insert(index + 1, newKey, value);
    }
    first() {
        return this.entryAt(0);
    }
    last() {
        return this.entryAt(-1);
    }
    clear() {
        this.#keys = [];
        return super.clear();
    }
    delete(key) {
        const deleted = super.delete(key);
        if (deleted) {
            this.#keys.splice(this.#keys.indexOf(key), 1);
        }
        return deleted;
    }
    deleteAt(index) {
        const key = this.keyAt(index);
        if (key !== void 0) {
            return this.delete(key);
        }
        return false;
    }
    at(index) {
        const key = at(this.#keys, index);
        if (key !== void 0) {
            return this.get(key);
        }
    }
    entryAt(index) {
        const key = at(this.#keys, index);
        if (key !== void 0) {
            return [
                key,
                this.get(key)
            ];
        }
    }
    indexOf(key) {
        return this.#keys.indexOf(key);
    }
    keyAt(index) {
        return at(this.#keys, index);
    }
    from(key, offset) {
        const index = this.indexOf(key);
        if (index === -1) {
            return void 0;
        }
        let dest = index + offset;
        if (dest < 0) dest = 0;
        if (dest >= this.size) dest = this.size - 1;
        return this.at(dest);
    }
    keyFrom(key, offset) {
        const index = this.indexOf(key);
        if (index === -1) {
            return void 0;
        }
        let dest = index + offset;
        if (dest < 0) dest = 0;
        if (dest >= this.size) dest = this.size - 1;
        return this.keyAt(dest);
    }
    find(predicate, thisArg) {
        let index = 0;
        for (const entry of this){
            if (Reflect.apply(predicate, thisArg, [
                entry,
                index,
                this
            ])) {
                return entry;
            }
            index++;
        }
        return void 0;
    }
    findIndex(predicate, thisArg) {
        let index = 0;
        for (const entry of this){
            if (Reflect.apply(predicate, thisArg, [
                entry,
                index,
                this
            ])) {
                return index;
            }
            index++;
        }
        return -1;
    }
    filter(predicate, thisArg) {
        const entries = [];
        let index = 0;
        for (const entry of this){
            if (Reflect.apply(predicate, thisArg, [
                entry,
                index,
                this
            ])) {
                entries.push(entry);
            }
            index++;
        }
        return new _OrderedDict(entries);
    }
    map(callbackfn, thisArg) {
        const entries = [];
        let index = 0;
        for (const entry of this){
            entries.push([
                entry[0],
                Reflect.apply(callbackfn, thisArg, [
                    entry,
                    index,
                    this
                ])
            ]);
            index++;
        }
        return new _OrderedDict(entries);
    }
    reduce(...args) {
        const [callbackfn, initialValue] = args;
        let index = 0;
        let accumulator = initialValue ?? this.at(0);
        for (const entry of this){
            if (index === 0 && args.length === 1) {
                accumulator = entry;
            } else {
                accumulator = Reflect.apply(callbackfn, this, [
                    accumulator,
                    entry,
                    index,
                    this
                ]);
            }
            index++;
        }
        return accumulator;
    }
    reduceRight(...args) {
        const [callbackfn, initialValue] = args;
        let accumulator = initialValue ?? this.at(-1);
        for(let index = this.size - 1; index >= 0; index--){
            const entry = this.at(index);
            if (index === this.size - 1 && args.length === 1) {
                accumulator = entry;
            } else {
                accumulator = Reflect.apply(callbackfn, this, [
                    accumulator,
                    entry,
                    index,
                    this
                ]);
            }
        }
        return accumulator;
    }
    toSorted(compareFn) {
        const entries = [
            ...this.entries()
        ].sort(compareFn);
        return new _OrderedDict(entries);
    }
    toReversed() {
        const reversed = new _OrderedDict();
        for(let index = this.size - 1; index >= 0; index--){
            const key = this.keyAt(index);
            const element = this.get(key);
            reversed.set(key, element);
        }
        return reversed;
    }
    toSpliced(...args) {
        const entries = [
            ...this.entries()
        ];
        entries.splice(...args);
        return new _OrderedDict(entries);
    }
    slice(start, end) {
        const result = new _OrderedDict();
        let stop = this.size - 1;
        if (start === void 0) {
            return result;
        }
        if (start < 0) {
            start = start + this.size;
        }
        if (end !== void 0 && end > 0) {
            stop = end - 1;
        }
        for(let index = start; index <= stop; index++){
            const key = this.keyAt(index);
            const element = this.get(key);
            result.set(key, element);
        }
        return result;
    }
    every(predicate, thisArg) {
        let index = 0;
        for (const entry of this){
            if (!Reflect.apply(predicate, thisArg, [
                entry,
                index,
                this
            ])) {
                return false;
            }
            index++;
        }
        return true;
    }
    some(predicate, thisArg) {
        let index = 0;
        for (const entry of this){
            if (Reflect.apply(predicate, thisArg, [
                entry,
                index,
                this
            ])) {
                return true;
            }
            index++;
        }
        return false;
    }
};
function at(array, index) {
    if ("at" in Array.prototype) {
        return Array.prototype.at.call(array, index);
    }
    const actualIndex = toSafeIndex(array, index);
    return actualIndex === -1 ? void 0 : array[actualIndex];
}
function toSafeIndex(array, index) {
    const length = array.length;
    const relativeIndex = toSafeInteger(index);
    const actualIndex = relativeIndex >= 0 ? relativeIndex : length + relativeIndex;
    return actualIndex < 0 || actualIndex >= length ? -1 : actualIndex;
}
function toSafeInteger(number) {
    return number !== number || number === 0 ? 0 : Math.trunc(number);
}
// src/collection.tsx

function createCollection2(name) {
    const PROVIDER_NAME = name + "CollectionProvider";
    const [createCollectionContext, createCollectionScope] = createContextScope2(PROVIDER_NAME);
    const [CollectionContextProvider, useCollectionContext] = createCollectionContext(PROVIDER_NAME, {
        collectionElement: null,
        collectionRef: {
            current: null
        },
        collectionRefObject: {
            current: null
        },
        itemMap: new OrderedDict(),
        setItemMap: ()=>void 0
    });
    const CollectionProvider = ({ state, ...props })=>{
        return state ? /* @__PURE__ */ jsx2(CollectionProviderImpl, {
            ...props,
            state
        }) : /* @__PURE__ */ jsx2(CollectionInit, {
            ...props
        });
    };
    CollectionProvider.displayName = PROVIDER_NAME;
    const CollectionInit = (props)=>{
        const state = useInitCollection();
        return /* @__PURE__ */ jsx2(CollectionProviderImpl, {
            ...props,
            state
        });
    };
    CollectionInit.displayName = PROVIDER_NAME + "Init";
    const CollectionProviderImpl = (props)=>{
        const { scope, children, state } = props;
        const ref = React2.useRef(null);
        const [collectionElement, setCollectionElement] = React2.useState(null);
        const composeRefs = useComposedRefs2(ref, setCollectionElement);
        const [itemMap, setItemMap] = state;
        React2.useEffect(()=>{
            if (!collectionElement) return;
            const observer = getChildListObserver(()=>{});
            observer.observe(collectionElement, {
                childList: true,
                subtree: true
            });
            return ()=>{
                observer.disconnect();
            };
        }, [
            collectionElement
        ]);
        return /* @__PURE__ */ jsx2(CollectionContextProvider, {
            scope,
            itemMap,
            setItemMap,
            collectionRef: composeRefs,
            collectionRefObject: ref,
            collectionElement,
            children
        });
    };
    CollectionProviderImpl.displayName = PROVIDER_NAME + "Impl";
    const COLLECTION_SLOT_NAME = name + "CollectionSlot";
    const CollectionSlotImpl = createSlot2(COLLECTION_SLOT_NAME);
    const CollectionSlot = /*#__PURE__*/ React2.forwardRef((props, forwardedRef)=>{
        const { scope, children } = props;
        const context = useCollectionContext(COLLECTION_SLOT_NAME, scope);
        const composedRefs = useComposedRefs2(forwardedRef, context.collectionRef);
        return /* @__PURE__ */ jsx2(CollectionSlotImpl, {
            ref: composedRefs,
            children
        });
    });
    CollectionSlot.displayName = COLLECTION_SLOT_NAME;
    const ITEM_SLOT_NAME = name + "CollectionItemSlot";
    const ITEM_DATA_ATTR = "data-radix-collection-item";
    const CollectionItemSlotImpl = createSlot2(ITEM_SLOT_NAME);
    const CollectionItemSlot = /*#__PURE__*/ React2.forwardRef((props, forwardedRef)=>{
        const { scope, children, ...itemData } = props;
        const ref = React2.useRef(null);
        const [element, setElement] = React2.useState(null);
        const composedRefs = useComposedRefs2(forwardedRef, ref, setElement);
        const context = useCollectionContext(ITEM_SLOT_NAME, scope);
        const { setItemMap } = context;
        const itemDataRef = React2.useRef(itemData);
        if (!shallowEqual(itemDataRef.current, itemData)) {
            itemDataRef.current = itemData;
        }
        const memoizedItemData = itemDataRef.current;
        React2.useEffect(()=>{
            const itemData2 = memoizedItemData;
            setItemMap((map)=>{
                if (!element) {
                    return map;
                }
                if (!map.has(element)) {
                    map.set(element, {
                        ...itemData2,
                        element
                    });
                    return map.toSorted(sortByDocumentPosition);
                }
                return map.set(element, {
                    ...itemData2,
                    element
                }).toSorted(sortByDocumentPosition);
            });
            return ()=>{
                setItemMap((map)=>{
                    if (!element || !map.has(element)) {
                        return map;
                    }
                    map.delete(element);
                    return new OrderedDict(map);
                });
            };
        }, [
            element,
            memoizedItemData,
            setItemMap
        ]);
        return /* @__PURE__ */ jsx2(CollectionItemSlotImpl, {
            ...{
                [ITEM_DATA_ATTR]: ""
            },
            ref: composedRefs,
            children
        });
    });
    CollectionItemSlot.displayName = ITEM_SLOT_NAME;
    function useInitCollection() {
        return React2.useState(new OrderedDict());
    }
    function useCollection(scope) {
        const { itemMap } = useCollectionContext(name + "CollectionConsumer", scope);
        return itemMap;
    }
    const functions = {
        createCollectionScope,
        useCollection,
        useInitCollection
    };
    return [
        {
            Provider: CollectionProvider,
            Slot: CollectionSlot,
            ItemSlot: CollectionItemSlot
        },
        functions
    ];
}
function shallowEqual(a, b) {
    if (a === b) return true;
    if (typeof a !== "object" || typeof b !== "object") return false;
    if (a == null || b == null) return false;
    const keysA = Object.keys(a);
    const keysB = Object.keys(b);
    if (keysA.length !== keysB.length) return false;
    for (const key of keysA){
        if (!Object.prototype.hasOwnProperty.call(b, key)) return false;
        if (a[key] !== b[key]) return false;
    }
    return true;
}
function isElementPreceding(a, b) {
    return !!(b.compareDocumentPosition(a) & Node.DOCUMENT_POSITION_PRECEDING);
}
function sortByDocumentPosition(a, b) {
    return !a[1].element || !b[1].element ? 0 : isElementPreceding(a[1].element, b[1].element) ? -1 : 1;
}
function getChildListObserver(callback) {
    const observer = new MutationObserver((mutationsList)=>{
        for (const mutation of mutationsList){
            if (mutation.type === "childList") {
                callback();
                return;
            }
        }
    });
    return observer;
}
 //# sourceMappingURL=index.mjs.map

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@radix-ui+primitive@1.1.6/node_modules/@radix-ui/primitive/dist/index.mjs
var __defProp = Object.defineProperty;
var __name = (target, value) => __defProp(target, "name", { value, configurable: true });

// src/primitive.tsx
var canUseDOM = !!(typeof window !== "undefined" && window.document && window.document.createElement);
function composeEventHandlers(originalEventHandler, ourEventHandler, { checkForDefaultPrevented = true } = {}) {
  return /* @__PURE__ */ __name(function handleEvent(event) {
    originalEventHandler?.(event);
    if (checkForDefaultPrevented === false || !event || !event.defaultPrevented) {
      return ourEventHandler?.(event);
    }
  }, "handleEvent");
}
__name(composeEventHandlers, "composeEventHandlers");
function getOwnerWindow(element) {
  if (!canUseDOM) {
    throw new Error("Cannot access window outside of the DOM");
  }
  return element?.ownerDocument?.defaultView ?? window;
}
__name(getOwnerWindow, "getOwnerWindow");
function getOwnerDocument(element) {
  if (!canUseDOM) {
    throw new Error("Cannot access document outside of the DOM");
  }
  return element?.ownerDocument ?? document;
}
__name(getOwnerDocument, "getOwnerDocument");
function getActiveElement(node, activeDescendant = false) {
  const { activeElement } = getOwnerDocument(node);
  if (!activeElement?.nodeName) {
    return null;
  }
  if (isFrame(activeElement) && activeElement.contentDocument) {
    return getActiveElement(activeElement.contentDocument.body, activeDescendant);
  }
  if (activeDescendant) {
    const id = activeElement.getAttribute("aria-activedescendant");
    if (id) {
      const element = getOwnerDocument(activeElement).getElementById(id);
      if (element) {
        return element;
      }
    }
  }
  return activeElement;
}
__name(getActiveElement, "getActiveElement");
function isFrame(element) {
  return element.tagName === "IFRAME";
}
__name(isFrame, "isFrame");

//# sourceMappingURL=index.mjs.map

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@radix-ui+primitive@1.1.6/node_modules/@radix-ui/primitive/dist/internal/is-development.false.mjs
// src/internal/is-development.false.ts
var IS_DEVELOPMENT = false;

//# sourceMappingURL=is-development.false.mjs.map

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@radix-ui+react-use-layout-effect@1.1.2+e8bcf8c23599d818/node_modules/@radix-ui/react-use-layout-effect/dist/index.mjs
// src/use-layout-effect.tsx

var useLayoutEffect2 = globalThis?.document ? react.useLayoutEffect : () => {
};

//# sourceMappingURL=index.mjs.map

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@radix-ui+react-use-effect-event@0.0.3+e8bcf8c23599d818/node_modules/@radix-ui/react-use-effect-event/dist/index.mjs
// src/use-effect-event.tsx


var useReactEffectEvent = react_namespaceObject[" useEffectEvent ".trim().toString()];
var useReactInsertionEffect = react_namespaceObject[" useInsertionEffect ".trim().toString()];
function useEffectEvent(callback) {
  if (typeof useReactEffectEvent === "function") {
    return useReactEffectEvent(callback);
  }
  const ref = react.useRef(() => {
    throw new Error("Cannot call an event handler while rendering.");
  });
  if (typeof useReactInsertionEffect === "function") {
    useReactInsertionEffect(() => {
      ref.current = callback;
    });
  } else {
    useLayoutEffect2(() => {
      ref.current = callback;
    });
  }
  return react.useMemo(() => ((...args) => ref.current?.(...args)), []);
}

//# sourceMappingURL=index.mjs.map

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@radix-ui+react-use-controllable-state@1.2.4+e8bcf8c23599d818/node_modules/@radix-ui/react-use-controllable-state/dist/index.mjs
var dist_defProp = Object.defineProperty;
var dist_name = (target, value) => dist_defProp(target, "name", { value, configurable: true });

// src/use-controllable-state.tsx



var useInsertionEffect = react_namespaceObject[" useInsertionEffect ".trim().toString()] || useLayoutEffect2;
function useControllableState({
  prop,
  defaultProp,
  onChange = /* @__PURE__ */ dist_name(() => {
  }, "onChange"),
  caller
}) {
  const [uncontrolledProp, setUncontrolledProp, onChangeRef] = useUncontrolledState({
    defaultProp,
    onChange
  });
  const isControlled = prop !== void 0;
  const value = isControlled ? prop : uncontrolledProp;
  if (IS_DEVELOPMENT) {
    const isControlledRef = react.useRef(prop !== void 0);
    react.useEffect(() => {
      const wasControlled = isControlledRef.current;
      if (wasControlled !== isControlled) {
        const from = wasControlled ? "controlled" : "uncontrolled";
        const to = isControlled ? "controlled" : "uncontrolled";
        console.warn(
          `${caller} is changing from ${from} to ${to}. Components should not switch from controlled to uncontrolled (or vice versa). Decide between using a controlled or uncontrolled value for the lifetime of the component.`
        );
      }
      isControlledRef.current = isControlled;
    }, [isControlled, caller]);
  }
  const setValue = react.useCallback(
    (nextValue) => {
      if (isControlled) {
        const value2 = isFunction(nextValue) ? nextValue(prop) : nextValue;
        if (value2 !== prop) {
          onChangeRef.current?.(value2);
        }
      } else {
        setUncontrolledProp(nextValue);
      }
    },
    [isControlled, prop, setUncontrolledProp, onChangeRef]
  );
  return [value, setValue];
}
dist_name(useControllableState, "useControllableState");
function useUncontrolledState({
  defaultProp,
  onChange
}) {
  const [value, setValue] = react.useState(defaultProp);
  const prevValueRef = react.useRef(value);
  const onChangeRef = react.useRef(onChange);
  useInsertionEffect(() => {
    onChangeRef.current = onChange;
  }, [onChange]);
  react.useEffect(() => {
    if (prevValueRef.current !== value) {
      onChangeRef.current?.(value);
      prevValueRef.current = value;
    }
  }, [value, prevValueRef]);
  return [value, setValue, onChangeRef];
}
dist_name(useUncontrolledState, "useUncontrolledState");
function isFunction(value) {
  return typeof value === "function";
}
dist_name(isFunction, "isFunction");

// src/use-controllable-state-reducer.tsx



var SYNC_STATE = Symbol("RADIX:SYNC_STATE");
function useControllableStateReducer(reducer, userArgs, initialArg, init) {
  const { prop: controlledState, defaultProp, onChange: onChangeProp, caller } = userArgs;
  const isControlled = controlledState !== void 0;
  const onChange = useEffectEvent(onChangeProp);
  if (IS_DEVELOPMENT) {
    const isControlledRef = react.useRef(controlledState !== void 0);
    react.useEffect(() => {
      const wasControlled = isControlledRef.current;
      if (wasControlled !== isControlled) {
        const from = wasControlled ? "controlled" : "uncontrolled";
        const to = isControlled ? "controlled" : "uncontrolled";
        console.warn(
          `${caller} is changing from ${from} to ${to}. Components should not switch from controlled to uncontrolled (or vice versa). Decide between using a controlled or uncontrolled value for the lifetime of the component.`
        );
      }
      isControlledRef.current = isControlled;
    }, [isControlled, caller]);
  }
  const args = [{ ...initialArg, state: defaultProp }];
  if (init) {
    args.push(init);
  }
  const [internalState, dispatch] = react.useReducer(
    (state2, action) => {
      if (action.type === SYNC_STATE) {
        return { ...state2, state: action.state };
      }
      const next = reducer(state2, action);
      if (isControlled && !Object.is(next.state, state2.state)) {
        onChange(next.state);
      }
      return next;
    },
    ...args
  );
  const uncontrolledState = internalState.state;
  const prevValueRef = react.useRef(uncontrolledState);
  react.useEffect(() => {
    if (prevValueRef.current !== uncontrolledState) {
      prevValueRef.current = uncontrolledState;
      if (!isControlled) {
        onChange(uncontrolledState);
      }
    }
  }, [uncontrolledState, prevValueRef, isControlled]);
  const state = react.useMemo(() => {
    const isControlled2 = controlledState !== void 0;
    if (isControlled2) {
      return { ...internalState, state: controlledState };
    }
    return internalState;
  }, [internalState, controlledState]);
  react.useEffect(() => {
    if (isControlled && !Object.is(controlledState, internalState.state)) {
      dispatch({ type: SYNC_STATE, state: controlledState });
    }
  }, [controlledState, internalState.state, isControlled]);
  return [state, dispatch];
}
dist_name(useControllableStateReducer, "useControllableStateReducer");

//# sourceMappingURL=index.mjs.map

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react-dom.js
var react_dom = __webpack_require__(68051);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@radix-ui+react-primitive@2.1.7+b721ee6ccd61acaa/node_modules/@radix-ui/react-primitive/dist/index.mjs
// src/primitive.tsx




var NODES = [
  "a",
  "button",
  "div",
  "form",
  "h2",
  "h3",
  "img",
  "input",
  "label",
  "li",
  "nav",
  "ol",
  "p",
  "select",
  "span",
  "svg",
  "ul"
];
var Primitive = NODES.reduce((primitive, node) => {
  const Slot = createSlot(`Primitive.${node}`);
  const Node = react.forwardRef((props, forwardedRef) => {
    const { asChild, ...primitiveProps } = props;
    const Comp = asChild ? Slot : node;
    if (typeof window !== "undefined") {
      window[Symbol.for("radix-ui")] = true;
    }
    return /* @__PURE__ */ (0,react_jsx_runtime.jsx)(Comp, { ...primitiveProps, ref: forwardedRef });
  });
  Node.displayName = `Primitive.${node}`;
  return { ...primitive, [node]: Node };
}, {});
function dispatchDiscreteCustomEvent(target, event) {
  if (target) ReactDOM.flushSync(() => target.dispatchEvent(event));
}
var Root = (/* unused pure expression or super */ null && (Primitive));

//# sourceMappingURL=index.mjs.map

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@radix-ui+react-presence@1.1.8+b721ee6ccd61acaa/node_modules/@radix-ui/react-presence/dist/index.mjs
/* __next_internal_client_entry_do_not_use__ Presence,Root auto */ var react_presence_dist_defProp = Object.defineProperty;
var react_presence_dist_name = (target, value)=>react_presence_dist_defProp(target, "name", {
        value,
        configurable: true
    });
// src/presence.tsx


// src/use-state-machine.tsx

function useStateMachine(initialState, machine) {
    return react.useReducer((state, event)=>{
        const nextState = machine[state][event];
        return nextState ?? state;
    }, initialState);
}
react_presence_dist_name(useStateMachine, "useStateMachine");
// src/presence.tsx
var Presence = /* @__PURE__ */ react_presence_dist_name((props)=>{
    const { present, children } = props;
    const presence = usePresence(present);
    const child = typeof children === "function" ? children({
        present: presence.isPresent
    }) : react.Children.only(children);
    const ref = useStableComposedRefs(presence.ref, dist_getElementRef(child));
    const forceMount = typeof children === "function";
    return forceMount || presence.isPresent ? /*#__PURE__*/ react.cloneElement(child, {
        ref
    }) : null;
}, "Presence");
function usePresence(present) {
    const [node, setNode] = react.useState();
    const stylesRef = react.useRef(null);
    const prevPresentRef = react.useRef(present);
    const prevAnimationNameRef = react.useRef("none");
    const mountAnimationNameRef = react.useRef(void 0);
    const initialState = present ? "mounted" : "unmounted";
    const [state, send] = useStateMachine(initialState, {
        mounted: {
            UNMOUNT: "unmounted",
            ANIMATION_OUT: "unmountSuspended"
        },
        unmountSuspended: {
            MOUNT: "mounted",
            ANIMATION_END: "unmounted"
        },
        unmounted: {
            MOUNT: "mounted"
        }
    });
    react.useEffect(()=>{
        if (state === "mounted") {
            prevAnimationNameRef.current = mountAnimationNameRef.current ?? getAnimationName(stylesRef.current);
            mountAnimationNameRef.current = void 0;
        } else {
            prevAnimationNameRef.current = "none";
        }
    }, [
        state
    ]);
    useLayoutEffect2(()=>{
        const styles = stylesRef.current;
        const wasPresent = prevPresentRef.current;
        const hasPresentChanged = wasPresent !== present;
        if (hasPresentChanged) {
            const prevAnimationName = prevAnimationNameRef.current;
            const currentAnimationName = getAnimationName(styles);
            if (present) {
                mountAnimationNameRef.current = currentAnimationName;
                send("MOUNT");
            } else if (currentAnimationName === "none" || styles?.display === "none") {
                send("UNMOUNT");
            } else {
                const isAnimating = prevAnimationName !== currentAnimationName;
                if (wasPresent && isAnimating) {
                    send("ANIMATION_OUT");
                } else {
                    send("UNMOUNT");
                }
            }
            prevPresentRef.current = present;
        }
    }, [
        present,
        send
    ]);
    useLayoutEffect2(()=>{
        if (node) {
            let timeoutId;
            const ownerWindow = node.ownerDocument.defaultView ?? window;
            const handleAnimationEnd = /* @__PURE__ */ react_presence_dist_name((event)=>{
                const currentAnimationName = getAnimationName(stylesRef.current);
                const isCurrentAnimation = currentAnimationName.includes(CSS.escape(event.animationName));
                if (event.target === node && isCurrentAnimation) {
                    send("ANIMATION_END");
                    if (!prevPresentRef.current) {
                        const currentFillMode = node.style.animationFillMode;
                        node.style.animationFillMode = "forwards";
                        timeoutId = ownerWindow.setTimeout(()=>{
                            if (node.style.animationFillMode === "forwards") {
                                node.style.animationFillMode = currentFillMode;
                            }
                        });
                    }
                }
            }, "handleAnimationEnd");
            const handleAnimationStart = /* @__PURE__ */ react_presence_dist_name((event)=>{
                if (event.target === node) {
                    prevAnimationNameRef.current = getAnimationName(stylesRef.current);
                }
            }, "handleAnimationStart");
            node.addEventListener("animationstart", handleAnimationStart);
            node.addEventListener("animationcancel", handleAnimationEnd);
            node.addEventListener("animationend", handleAnimationEnd);
            return ()=>{
                ownerWindow.clearTimeout(timeoutId);
                node.removeEventListener("animationstart", handleAnimationStart);
                node.removeEventListener("animationcancel", handleAnimationEnd);
                node.removeEventListener("animationend", handleAnimationEnd);
            };
        } else {
            send("ANIMATION_END");
        }
    }, [
        node,
        send
    ]);
    return {
        isPresent: [
            "mounted",
            "unmountSuspended"
        ].includes(state),
        ref: react.useCallback((node2)=>{
            if (node2) {
                const styles = getComputedStyle(node2);
                stylesRef.current = styles;
                mountAnimationNameRef.current = getAnimationName(styles);
            } else {
                stylesRef.current = null;
            }
            setNode(node2);
        }, [])
    };
}
react_presence_dist_name(usePresence, "usePresence");
function dist_setRef(ref, value) {
    if (typeof ref === "function") {
        return ref(value);
    } else if (ref !== null && ref !== void 0) {
        ref.current = value;
    }
}
react_presence_dist_name(dist_setRef, "setRef");
function useStableComposedRefs(...refs) {
    const refsRef = react.useRef(refs);
    refsRef.current = refs;
    return react.useCallback((node)=>{
        const currentRefs = refsRef.current;
        let hasCleanup = false;
        const cleanups = currentRefs.map((ref)=>{
            const cleanup = dist_setRef(ref, node);
            if (!hasCleanup && typeof cleanup === "function") {
                hasCleanup = true;
            }
            return cleanup;
        });
        if (hasCleanup) {
            return ()=>{
                for(let i = 0; i < cleanups.length; i++){
                    const cleanup = cleanups[i];
                    if (typeof cleanup === "function") {
                        cleanup();
                    } else {
                        dist_setRef(currentRefs[i], null);
                    }
                }
            };
        }
    }, []);
}
react_presence_dist_name(useStableComposedRefs, "useStableComposedRefs");
function getAnimationName(styles) {
    return styles?.animationName || "none";
}
react_presence_dist_name(getAnimationName, "getAnimationName");
function dist_getElementRef(element) {
    let getter = Object.getOwnPropertyDescriptor(element.props, "ref")?.get;
    let mayWarn = getter && "isReactWarning" in getter && getter.isReactWarning;
    if (mayWarn) {
        return element.ref;
    }
    getter = Object.getOwnPropertyDescriptor(element, "ref")?.get;
    mayWarn = getter && "isReactWarning" in getter && getter.isReactWarning;
    if (mayWarn) {
        return element.props.ref;
    }
    return element.props.ref || element.ref;
}
react_presence_dist_name(dist_getElementRef, "getElementRef");
var dist_Root = (/* unused pure expression or super */ null && (Presence));
 //# sourceMappingURL=index.mjs.map

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@radix-ui+react-id@1.1.2+e8bcf8c23599d818/node_modules/@radix-ui/react-id/dist/index.mjs
// src/id.tsx


var useReactId = react_namespaceObject[" useId ".trim().toString()] || (() => void 0);
var count = 0;
function useId(deterministicId) {
  const [id, setId] = react.useState(useReactId());
  useLayoutEffect2(() => {
    if (!deterministicId) setId((reactId) => reactId ?? String(count++));
  }, [deterministicId]);
  return deterministicId || (id ? `radix-${id}` : "");
}

//# sourceMappingURL=index.mjs.map

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@radix-ui+react-collapsible@1.1.17+b721ee6ccd61acaa/node_modules/@radix-ui/react-collapsible/dist/index.mjs
/* __next_internal_client_entry_do_not_use__ Collapsible,CollapsibleContent,CollapsibleTrigger,Content,Root,Trigger,createCollapsibleScope auto */ var react_collapsible_dist_defProp = Object.defineProperty;
var react_collapsible_dist_name = (target, value)=>react_collapsible_dist_defProp(target, "name", {
        value,
        configurable: true
    });
// src/collapsible.tsx










var COLLAPSIBLE_NAME = "Collapsible";
var [createCollapsibleContext, createCollapsibleScope] = createContextScope(COLLAPSIBLE_NAME);
var [CollapsibleProvider, useCollapsibleContext] = createCollapsibleContext(COLLAPSIBLE_NAME);
var Collapsible = /* @__PURE__ */ /*#__PURE__*/ react.forwardRef(/* @__PURE__ */ react_collapsible_dist_name(function Collapsible2(props, forwardedRef) {
    const { __scopeCollapsible, open: openProp, defaultOpen, disabled, onOpenChange, ...collapsibleProps } = props;
    const [open, setOpen] = useControllableState({
        prop: openProp,
        defaultProp: defaultOpen ?? false,
        onChange: onOpenChange,
        caller: COLLAPSIBLE_NAME
    });
    return /* @__PURE__ */ (0,react_jsx_runtime.jsx)(CollapsibleProvider, {
        scope: __scopeCollapsible,
        disabled,
        contentId: useId(),
        open,
        onOpenToggle: react.useCallback(()=>setOpen((prevOpen)=>!prevOpen), [
            setOpen
        ]),
        children: /* @__PURE__ */ (0,react_jsx_runtime.jsx)(Primitive.div, {
            "data-state": getState(open),
            "data-disabled": disabled ? "" : void 0,
            ...collapsibleProps,
            ref: forwardedRef
        })
    });
}, "Collapsible"));
var TRIGGER_NAME = "CollapsibleTrigger";
var CollapsibleTrigger = /* @__PURE__ */ /*#__PURE__*/ react.forwardRef(// blank line to reduce diff noise
/* @__PURE__ */ react_collapsible_dist_name(function CollapsibleTrigger2(props, forwardedRef) {
    const { __scopeCollapsible, ...triggerProps } = props;
    const context = useCollapsibleContext(TRIGGER_NAME, __scopeCollapsible);
    return /* @__PURE__ */ (0,react_jsx_runtime.jsx)(Primitive.button, {
        type: "button",
        "aria-controls": context.open ? context.contentId : void 0,
        "aria-expanded": context.open || false,
        "data-state": getState(context.open),
        "data-disabled": context.disabled ? "" : void 0,
        disabled: context.disabled,
        ...triggerProps,
        ref: forwardedRef,
        onClick: composeEventHandlers(props.onClick, context.onOpenToggle)
    });
}, "CollapsibleTrigger"));
var CONTENT_NAME = "CollapsibleContent";
var CollapsibleContent = /* @__PURE__ */ /*#__PURE__*/ react.forwardRef(// blank line to reduce diff noise
/* @__PURE__ */ react_collapsible_dist_name(function CollapsibleContent2(props, forwardedRef) {
    const { forceMount, ...contentProps } = props;
    const context = useCollapsibleContext(CONTENT_NAME, props.__scopeCollapsible);
    return /* @__PURE__ */ (0,react_jsx_runtime.jsx)(Presence, {
        present: forceMount || context.open,
        children: ({ present })=>/* @__PURE__ */ (0,react_jsx_runtime.jsx)(CollapsibleContentImpl, {
                ...contentProps,
                ref: forwardedRef,
                present
            })
    });
}, "CollapsibleContent"));
var CollapsibleContentImpl = /* @__PURE__ */ /*#__PURE__*/ react.forwardRef(/* @__PURE__ */ react_collapsible_dist_name(function CollapsibleContentImpl2(props, forwardedRef) {
    const { __scopeCollapsible, present, children, ...contentProps } = props;
    const context = useCollapsibleContext(CONTENT_NAME, __scopeCollapsible);
    const [isPresent, setIsPresent] = react.useState(present);
    const ref = react.useRef(null);
    const composedRefs = useComposedRefs(forwardedRef, ref);
    const heightRef = react.useRef(0);
    const height = heightRef.current;
    const widthRef = react.useRef(0);
    const width = widthRef.current;
    const isOpen = context.open || isPresent;
    const isMountAnimationPreventedRef = react.useRef(isOpen);
    const originalStylesRef = react.useRef(void 0);
    react.useEffect(()=>{
        const rAF = requestAnimationFrame(()=>isMountAnimationPreventedRef.current = false);
        return ()=>cancelAnimationFrame(rAF);
    }, []);
    useLayoutEffect2(()=>{
        const node = ref.current;
        if (node) {
            originalStylesRef.current = originalStylesRef.current || {
                transitionDuration: node.style.transitionDuration,
                animationName: node.style.animationName
            };
            node.style.transitionDuration = "0s";
            node.style.animationName = "none";
            const rect = node.getBoundingClientRect();
            heightRef.current = rect.height;
            widthRef.current = rect.width;
            if (!isMountAnimationPreventedRef.current) {
                node.style.transitionDuration = originalStylesRef.current.transitionDuration;
                node.style.animationName = originalStylesRef.current.animationName;
            }
            setIsPresent(present);
        }
    }, [
        context.open,
        present
    ]);
    return /* @__PURE__ */ (0,react_jsx_runtime.jsx)(Primitive.div, {
        "data-state": getState(context.open),
        "data-disabled": context.disabled ? "" : void 0,
        id: context.contentId,
        hidden: !isOpen,
        ...contentProps,
        ref: composedRefs,
        style: {
            [`--radix-collapsible-content-height`]: height ? `${height}px` : void 0,
            [`--radix-collapsible-content-width`]: width ? `${width}px` : void 0,
            ...props.style
        },
        children: isOpen && children
    });
}, "CollapsibleContentImpl"));
function getState(open) {
    return open ? "open" : "closed";
}
react_collapsible_dist_name(getState, "getState");
var react_collapsible_dist_Root = Collapsible;
var Trigger = CollapsibleTrigger;
var Content = CollapsibleContent;
 //# sourceMappingURL=index.mjs.map

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@radix-ui+react-direction@1.1.2+e8bcf8c23599d818/node_modules/@radix-ui/react-direction/dist/index.mjs
/* __next_internal_client_entry_do_not_use__ DirectionProvider,Provider,useDirection auto */ // src/direction.tsx


var DirectionContext = /*#__PURE__*/ react.createContext(void 0);
var DirectionProvider = (props)=>{
    const { dir, children } = props;
    return /* @__PURE__ */ jsx(DirectionContext.Provider, {
        value: dir,
        children
    });
};
function useDirection(localDir) {
    const globalDir = react.useContext(DirectionContext);
    return localDir || globalDir || "ltr";
}
var Provider = (/* unused pure expression or super */ null && (DirectionProvider));
 //# sourceMappingURL=index.mjs.map

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@radix-ui+react-accordion@1.2.17+b721ee6ccd61acaa/node_modules/@radix-ui/react-accordion/dist/index.mjs
/* __next_internal_client_entry_do_not_use__ Accordion,AccordionContent,AccordionHeader,AccordionItem,AccordionTrigger,Content,Header,Item,Root,Trigger,createAccordionScope auto */ var react_accordion_dist_defProp = Object.defineProperty;
var react_accordion_dist_name = (target, value)=>react_accordion_dist_defProp(target, "name", {
        value,
        configurable: true
    });
// src/accordion.tsx












var ACCORDION_NAME = "Accordion";
var ACCORDION_KEYS = [
    "Home",
    "End",
    "ArrowDown",
    "ArrowUp",
    "ArrowLeft",
    "ArrowRight"
];
var [Collection, useCollection, createCollectionScope] = createCollection(ACCORDION_NAME);
var [createAccordionContext, createAccordionScope] = createContextScope(ACCORDION_NAME, [
    createCollectionScope,
    createCollapsibleScope
]);
var useCollapsibleScope = createCollapsibleScope();
var Accordion = /* @__PURE__ */ /*#__PURE__*/ react.forwardRef(// blank line to reduce diff noise
/* @__PURE__ */ react_accordion_dist_name(function Accordion2(props, forwardedRef) {
    const { type, ...accordionProps } = props;
    const singleProps = accordionProps;
    const multipleProps = accordionProps;
    return /* @__PURE__ */ (0,react_jsx_runtime.jsx)(Collection.Provider, {
        scope: props.__scopeAccordion,
        children: type === "multiple" ? /* @__PURE__ */ (0,react_jsx_runtime.jsx)(AccordionImplMultiple, {
            ...multipleProps,
            ref: forwardedRef
        }) : /* @__PURE__ */ (0,react_jsx_runtime.jsx)(AccordionImplSingle, {
            ...singleProps,
            ref: forwardedRef
        })
    });
}, "Accordion"));
var [AccordionValueProvider, useAccordionValueContext] = createAccordionContext(ACCORDION_NAME);
var [AccordionCollapsibleProvider, useAccordionCollapsibleContext] = createAccordionContext(ACCORDION_NAME, {
    collapsible: false
});
var AccordionImplSingle = /* @__PURE__ */ /*#__PURE__*/ react.forwardRef(// blank line to reduce diff noise
/* @__PURE__ */ react_accordion_dist_name(function AccordionImplSingle2(props, forwardedRef) {
    const { value: valueProp, defaultValue, onValueChange = /* @__PURE__ */ react_accordion_dist_name(()=>{}, "onValueChange"), collapsible = false, ...accordionSingleProps } = props;
    const [value, setValue] = useControllableState({
        prop: valueProp,
        defaultProp: defaultValue ?? "",
        onChange: onValueChange,
        caller: ACCORDION_NAME
    });
    return /* @__PURE__ */ (0,react_jsx_runtime.jsx)(AccordionValueProvider, {
        scope: props.__scopeAccordion,
        value: react.useMemo(()=>value ? [
                value
            ] : [], [
            value
        ]),
        onItemOpen: setValue,
        onItemClose: react.useCallback(()=>collapsible && setValue(""), [
            collapsible,
            setValue
        ]),
        children: /* @__PURE__ */ (0,react_jsx_runtime.jsx)(AccordionCollapsibleProvider, {
            scope: props.__scopeAccordion,
            collapsible,
            children: /* @__PURE__ */ (0,react_jsx_runtime.jsx)(AccordionImpl, {
                ...accordionSingleProps,
                ref: forwardedRef
            })
        })
    });
}, "AccordionImplSingle"));
var AccordionImplMultiple = /* @__PURE__ */ /*#__PURE__*/ react.forwardRef(/* @__PURE__ */ react_accordion_dist_name(function AccordionImplMultiple2(props, forwardedRef) {
    const { value: valueProp, defaultValue, onValueChange = /* @__PURE__ */ react_accordion_dist_name(()=>{}, "onValueChange"), ...accordionMultipleProps } = props;
    const [value, setValue] = useControllableState({
        prop: valueProp,
        defaultProp: defaultValue ?? [],
        onChange: onValueChange,
        caller: ACCORDION_NAME
    });
    const handleItemOpen = react.useCallback((itemValue)=>setValue((prevValue = [])=>[
                ...prevValue,
                itemValue
            ]), [
        setValue
    ]);
    const handleItemClose = react.useCallback((itemValue)=>setValue((prevValue = [])=>prevValue.filter((value2)=>value2 !== itemValue)), [
        setValue
    ]);
    return /* @__PURE__ */ (0,react_jsx_runtime.jsx)(AccordionValueProvider, {
        scope: props.__scopeAccordion,
        value,
        onItemOpen: handleItemOpen,
        onItemClose: handleItemClose,
        children: /* @__PURE__ */ (0,react_jsx_runtime.jsx)(AccordionCollapsibleProvider, {
            scope: props.__scopeAccordion,
            collapsible: true,
            children: /* @__PURE__ */ (0,react_jsx_runtime.jsx)(AccordionImpl, {
                ...accordionMultipleProps,
                ref: forwardedRef
            })
        })
    });
}, "AccordionImplMultiple"));
var [AccordionImplProvider, useAccordionContext] = createAccordionContext(ACCORDION_NAME);
var AccordionImpl = /* @__PURE__ */ /*#__PURE__*/ react.forwardRef(/* @__PURE__ */ react_accordion_dist_name(function AccordionImpl2(props, forwardedRef) {
    const { __scopeAccordion, disabled, dir, orientation = "vertical", ...accordionProps } = props;
    const accordionRef = react.useRef(null);
    const composedRefs = useComposedRefs(accordionRef, forwardedRef);
    const getItems = useCollection(__scopeAccordion);
    const direction = useDirection(dir);
    const isDirectionLTR = direction === "ltr";
    const handleKeyDown = composeEventHandlers(props.onKeyDown, (event)=>{
        if (!ACCORDION_KEYS.includes(event.key)) return;
        const target = event.target;
        const triggerCollection = getItems().filter((item)=>!item.ref.current?.disabled);
        const triggerIndex = triggerCollection.findIndex((item)=>item.ref.current === target);
        const triggerCount = triggerCollection.length;
        if (triggerIndex === -1) return;
        event.preventDefault();
        let nextIndex = triggerIndex;
        const homeIndex = 0;
        const endIndex = triggerCount - 1;
        const moveNext = /* @__PURE__ */ react_accordion_dist_name(()=>{
            nextIndex = triggerIndex + 1;
            if (nextIndex > endIndex) {
                nextIndex = homeIndex;
            }
        }, "moveNext");
        const movePrev = /* @__PURE__ */ react_accordion_dist_name(()=>{
            nextIndex = triggerIndex - 1;
            if (nextIndex < homeIndex) {
                nextIndex = endIndex;
            }
        }, "movePrev");
        switch(event.key){
            case "Home":
                nextIndex = homeIndex;
                break;
            case "End":
                nextIndex = endIndex;
                break;
            case "ArrowRight":
                if (orientation === "horizontal") {
                    if (isDirectionLTR) {
                        moveNext();
                    } else {
                        movePrev();
                    }
                }
                break;
            case "ArrowDown":
                if (orientation === "vertical") {
                    moveNext();
                }
                break;
            case "ArrowLeft":
                if (orientation === "horizontal") {
                    if (isDirectionLTR) {
                        movePrev();
                    } else {
                        moveNext();
                    }
                }
                break;
            case "ArrowUp":
                if (orientation === "vertical") {
                    movePrev();
                }
                break;
        }
        const clampedIndex = nextIndex % triggerCount;
        triggerCollection[clampedIndex].ref.current?.focus();
    });
    return /* @__PURE__ */ (0,react_jsx_runtime.jsx)(AccordionImplProvider, {
        scope: __scopeAccordion,
        disabled,
        direction: dir,
        orientation,
        children: /* @__PURE__ */ (0,react_jsx_runtime.jsx)(Collection.Slot, {
            scope: __scopeAccordion,
            children: /* @__PURE__ */ (0,react_jsx_runtime.jsx)(Primitive.div, {
                ...accordionProps,
                "data-orientation": orientation,
                ref: composedRefs,
                onKeyDown: disabled ? void 0 : handleKeyDown
            })
        })
    });
}, "AccordionImpl"));
var ITEM_NAME = "AccordionItem";
var [AccordionItemProvider, useAccordionItemContext] = createAccordionContext(ITEM_NAME);
var AccordionItem = /* @__PURE__ */ /*#__PURE__*/ react.forwardRef(/* @__PURE__ */ react_accordion_dist_name(function AccordionItem2(props, forwardedRef) {
    const { __scopeAccordion, value, ...accordionItemProps } = props;
    const accordionContext = useAccordionContext(ITEM_NAME, __scopeAccordion);
    const valueContext = useAccordionValueContext(ITEM_NAME, __scopeAccordion);
    const collapsibleScope = useCollapsibleScope(__scopeAccordion);
    const triggerId = useId();
    const open = value && valueContext.value.includes(value) || false;
    const disabled = accordionContext.disabled || props.disabled;
    return /* @__PURE__ */ (0,react_jsx_runtime.jsx)(AccordionItemProvider, {
        scope: __scopeAccordion,
        open,
        disabled,
        triggerId,
        children: /* @__PURE__ */ (0,react_jsx_runtime.jsx)(react_collapsible_dist_Root, {
            "data-orientation": accordionContext.orientation,
            "data-state": dist_getState(open),
            ...collapsibleScope,
            ...accordionItemProps,
            ref: forwardedRef,
            disabled,
            open,
            onOpenChange: (open2)=>{
                if (open2) {
                    valueContext.onItemOpen(value);
                } else {
                    valueContext.onItemClose(value);
                }
            }
        })
    });
}, "AccordionItem"));
var HEADER_NAME = "AccordionHeader";
var AccordionHeader = /* @__PURE__ */ /*#__PURE__*/ react.forwardRef(// blank line to reduce diff noise
/* @__PURE__ */ react_accordion_dist_name(function AccordionHeader2(props, forwardedRef) {
    const { __scopeAccordion, ...headerProps } = props;
    const accordionContext = useAccordionContext(ACCORDION_NAME, __scopeAccordion);
    const itemContext = useAccordionItemContext(HEADER_NAME, __scopeAccordion);
    return /* @__PURE__ */ (0,react_jsx_runtime.jsx)(Primitive.h3, {
        "data-orientation": accordionContext.orientation,
        "data-state": dist_getState(itemContext.open),
        "data-disabled": itemContext.disabled ? "" : void 0,
        ...headerProps,
        ref: forwardedRef
    });
}, "AccordionHeader"));
var dist_TRIGGER_NAME = "AccordionTrigger";
var AccordionTrigger = /* @__PURE__ */ /*#__PURE__*/ react.forwardRef(// blank line to reduce diff noise
/* @__PURE__ */ react_accordion_dist_name(function AccordionTrigger2(props, forwardedRef) {
    const { __scopeAccordion, ...triggerProps } = props;
    const accordionContext = useAccordionContext(ACCORDION_NAME, __scopeAccordion);
    const itemContext = useAccordionItemContext(dist_TRIGGER_NAME, __scopeAccordion);
    const collapsibleContext = useAccordionCollapsibleContext(dist_TRIGGER_NAME, __scopeAccordion);
    const collapsibleScope = useCollapsibleScope(__scopeAccordion);
    return /* @__PURE__ */ (0,react_jsx_runtime.jsx)(Collection.ItemSlot, {
        scope: __scopeAccordion,
        children: /* @__PURE__ */ (0,react_jsx_runtime.jsx)(Trigger, {
            "aria-disabled": itemContext.open && !collapsibleContext.collapsible || void 0,
            "data-orientation": accordionContext.orientation,
            id: itemContext.triggerId,
            ...collapsibleScope,
            ...triggerProps,
            ref: forwardedRef
        })
    });
}, "AccordionTrigger"));
var dist_CONTENT_NAME = "AccordionContent";
var AccordionContent = /* @__PURE__ */ /*#__PURE__*/ react.forwardRef(// blank line to reduce diff noise
/* @__PURE__ */ react_accordion_dist_name(function AccordionContent2(props, forwardedRef) {
    const { __scopeAccordion, ...contentProps } = props;
    const accordionContext = useAccordionContext(ACCORDION_NAME, __scopeAccordion);
    const itemContext = useAccordionItemContext(dist_CONTENT_NAME, __scopeAccordion);
    const collapsibleScope = useCollapsibleScope(__scopeAccordion);
    return /* @__PURE__ */ (0,react_jsx_runtime.jsx)(Content, {
        role: "region",
        "aria-labelledby": itemContext.triggerId,
        "data-orientation": accordionContext.orientation,
        ...collapsibleScope,
        ...contentProps,
        ref: forwardedRef,
        style: {
            "--radix-accordion-content-height": "var(--radix-collapsible-content-height)",
            "--radix-accordion-content-width": "var(--radix-collapsible-content-width)",
            ...props.style
        }
    });
}, "AccordionContent"));
function dist_getState(open) {
    return open ? "open" : "closed";
}
react_accordion_dist_name(dist_getState, "getState");
var Root2 = Accordion;
var Item = AccordionItem;
var Header = AccordionHeader;
var Trigger2 = AccordionTrigger;
var Content2 = AccordionContent;
 //# sourceMappingURL=index.mjs.map


/***/ }),

/***/ 97503:
/***/ ((module, __unused_webpack___webpack_exports__, __webpack_require__) => {

/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(40934);
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__);
// Imports

var ___CSS_LOADER_EXPORT___ = _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default()(false);
// Module
___CSS_LOADER_EXPORT___.push([module.id, "@keyframes _1nh0oet4 {\n  from {\n    height: 0;\n  }\n  to {\n    height: var(--radix-accordion-content-height);\n  }\n}\n@keyframes _1nh0oet5 {\n  from {\n    height: var(--radix-accordion-content-height);\n  }\n  to {\n    height: 0;\n  }\n}\n._1nh0oet0 {\n  width: 100%;\n}\n._1nh0oet1 {\n  border-bottom: 1px solid var(--_19ez2rze);\n}\n._1nh0oet1:last-child {\n  border-bottom: none;\n}\n._1nh0oet2 {\n  display: flex;\n  width: 100%;\n  min-height: 44px;\n  align-items: center;\n  justify-content: space-between;\n  gap: var(--_19ez2rz2z);\n  padding: var(--_19ez2rz30) var(--_19ez2rz2z);\n  background: transparent;\n  border: none;\n  cursor: pointer;\n  color: var(--_19ez2rz0);\n  font-size: var(--_19ez2rz3c);\n  font-weight: var(--_19ez2rz3h);\n  text-align: left;\n}\n._1nh0oet2:hover {\n  background: var(--_19ez2rz8);\n}\n._1nh0oet2:focus-visible {\n  outline: 2px solid var(--_19ez2rzm);\n  outline-offset: -2px;\n}\n._1nh0oet3 {\n  display: inline-flex;\n  align-items: center;\n  justify-content: center;\n  flex-shrink: 0;\n  transition: transform 0.15s ease;\n  color: var(--_19ez2rz2);\n}\n._1nh0oet2[data-state=\"open\"] ._1nh0oet3 {\n  transform: rotate(90deg);\n}\n._1nh0oet6 {\n  overflow: hidden;\n}\n._1nh0oet6[data-state=\"open\"] {\n  animation: _1nh0oet4 0.15s ease-out;\n}\n._1nh0oet6[data-state=\"closed\"] {\n  animation: _1nh0oet5 0.15s ease-out;\n}\n._1nh0oet7 {\n  padding: 0 var(--_19ez2rz2z) var(--_19ez2rz30);\n}", ""]);
// Exports
/* unused harmony default export */ var __WEBPACK_DEFAULT_EXPORT__ = ((/* unused pure expression or super */ null && (___CSS_LOADER_EXPORT___)));


/***/ })

};
;