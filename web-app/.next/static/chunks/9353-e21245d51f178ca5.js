"use strict";
(self["webpackChunk_N_E"] = self["webpackChunk_N_E"] || []).push([[9353],{

/***/ 1844:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   w: () => (/* binding */ fileDesc)
/* harmony export */ });
/* harmony import */ var _wire_base64_encoding_js__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(14097);
/* harmony import */ var _wkt_gen_google_protobuf_descriptor_pb_js__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(8461);
/* harmony import */ var _registry_js__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(31503);
/* harmony import */ var _restore_json_names_js__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(12140);
/* harmony import */ var _from_binary_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(57386);
// Copyright 2021-2026 Buf Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.





/**
 * Hydrate a file descriptor.
 *
 * @private
 */
function fileDesc(b64, imports) {
    var _a;
    const root = (0,_from_binary_js__WEBPACK_IMPORTED_MODULE_0__/* .fromBinary */ .Er)(_wkt_gen_google_protobuf_descriptor_pb_js__WEBPACK_IMPORTED_MODULE_1__/* .FileDescriptorProtoSchema */ .tr, (0,_wire_base64_encoding_js__WEBPACK_IMPORTED_MODULE_2__/* .base64Decode */ .u)(b64));
    root.messageType.forEach(_restore_json_names_js__WEBPACK_IMPORTED_MODULE_3__/* .restoreJsonNames */ .h);
    root.dependency = (_a = imports === null || imports === void 0 ? void 0 : imports.map((f) => f.proto.name)) !== null && _a !== void 0 ? _a : [];
    const reg = (0,_registry_js__WEBPACK_IMPORTED_MODULE_4__/* .createFileRegistry */ .uO)(root, (protoFileName) => imports === null || imports === void 0 ? void 0 : imports.find((f) => f.proto.name === protoFileName));
    // biome-ignore lint/style/noNonNullAssertion: non-null assertion because we just created the registry from the file we look up
    return reg.getFile(root.name);
}


/***/ }),

/***/ 2681:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   R: () => (/* binding */ toBinary),
/* harmony export */   Y: () => (/* binding */ writeField)
/* harmony export */ });
/* harmony import */ var _reflect_reflect_js__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(64683);
/* harmony import */ var _wire_binary_encoding_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(26811);
/* harmony import */ var _descriptors_js__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(92806);
// Copyright 2021-2026 Buf Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.



// bootstrap-inject google.protobuf.FeatureSet.FieldPresence.LEGACY_REQUIRED: const $name: FeatureSet_FieldPresence.$localName = $number;
const LEGACY_REQUIRED = 3;
// Default options for serializing binary data.
const writeDefaults = {
    writeUnknownFields: true,
};
function makeWriteOptions(options) {
    return options ? Object.assign(Object.assign({}, writeDefaults), options) : writeDefaults;
}
function toBinary(schema, message, options) {
    return writeFields(new _wire_binary_encoding_js__WEBPACK_IMPORTED_MODULE_0__/* .BinaryWriter */ .fI(), makeWriteOptions(options), (0,_reflect_reflect_js__WEBPACK_IMPORTED_MODULE_1__/* .reflect */ .E4)(schema, message)).finish();
}
function writeFields(writer, opts, msg) {
    var _a;
    for (const f of msg.sortedFields) {
        if (!msg.isSet(f)) {
            if (f.presence == LEGACY_REQUIRED) {
                throw new Error(`cannot encode ${f} to binary: required field not set`);
            }
            continue;
        }
        writeField(writer, opts, msg, f);
    }
    if (opts.writeUnknownFields) {
        for (const { no, wireType, data } of (_a = msg.getUnknown()) !== null && _a !== void 0 ? _a : []) {
            writer.tag(no, wireType).raw(data);
        }
    }
    return writer;
}
/**
 * @private
 */
function writeField(writer, opts, msg, field) {
    var _a;
    switch (field.fieldKind) {
        case "scalar":
        case "enum":
            writeScalar(writer, msg.desc.typeName, field.name, (_a = field.scalar) !== null && _a !== void 0 ? _a : _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.INT32, field.number, msg.get(field));
            break;
        case "list":
            writeListField(writer, opts, field, msg.get(field));
            break;
        case "message":
            writeMessageField(writer, opts, field, msg.get(field));
            break;
        case "map":
            for (const [key, val] of msg.get(field)) {
                writeMapEntry(writer, opts, field, key, val);
            }
            break;
    }
}
function writeScalar(writer, msgName, fieldName, scalarType, fieldNo, value) {
    writeScalarValue(writer.tag(fieldNo, writeTypeOfScalar(scalarType)), msgName, fieldName, scalarType, value);
}
function writeMessageField(writer, opts, field, message) {
    if (field.delimitedEncoding) {
        writeFields(writer.tag(field.number, _wire_binary_encoding_js__WEBPACK_IMPORTED_MODULE_0__/* .WireType */ .O0.StartGroup), opts, message).tag(field.number, _wire_binary_encoding_js__WEBPACK_IMPORTED_MODULE_0__/* .WireType */ .O0.EndGroup);
    }
    else {
        writeFields(writer.tag(field.number, _wire_binary_encoding_js__WEBPACK_IMPORTED_MODULE_0__/* .WireType */ .O0.LengthDelimited).fork(), opts, message).join();
    }
}
function writeListField(writer, opts, field, list) {
    var _a;
    if (field.listKind == "message") {
        for (const item of list) {
            writeMessageField(writer, opts, field, item);
        }
        return;
    }
    const scalarType = (_a = field.scalar) !== null && _a !== void 0 ? _a : _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.INT32;
    if (field.packed) {
        if (!list.size) {
            return;
        }
        writer.tag(field.number, _wire_binary_encoding_js__WEBPACK_IMPORTED_MODULE_0__/* .WireType */ .O0.LengthDelimited).fork();
        for (const item of list) {
            writeScalarValue(writer, field.parent.typeName, field.name, scalarType, item);
        }
        writer.join();
        return;
    }
    for (const item of list) {
        writeScalar(writer, field.parent.typeName, field.name, scalarType, field.number, item);
    }
}
function writeMapEntry(writer, opts, field, key, value) {
    var _a;
    writer.tag(field.number, _wire_binary_encoding_js__WEBPACK_IMPORTED_MODULE_0__/* .WireType */ .O0.LengthDelimited).fork();
    // write key, expecting key field number = 1
    writeScalar(writer, field.parent.typeName, field.name, field.mapKey, 1, key);
    // write value, expecting value field number = 2
    switch (field.mapKind) {
        case "scalar":
        case "enum":
            writeScalar(writer, field.parent.typeName, field.name, (_a = field.scalar) !== null && _a !== void 0 ? _a : _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.INT32, 2, value);
            break;
        case "message":
            writeFields(writer.tag(2, _wire_binary_encoding_js__WEBPACK_IMPORTED_MODULE_0__/* .WireType */ .O0.LengthDelimited).fork(), opts, value).join();
            break;
    }
    writer.join();
}
function writeScalarValue(writer, msgName, fieldName, type, value) {
    try {
        switch (type) {
            case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.STRING:
                writer.string(value);
                break;
            case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.BOOL:
                writer.bool(value);
                break;
            case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.DOUBLE:
                writer.double(value);
                break;
            case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.FLOAT:
                writer.float(value);
                break;
            case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.INT32:
                writer.int32(value);
                break;
            case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.INT64:
                writer.int64(value);
                break;
            case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.UINT64:
                writer.uint64(value);
                break;
            case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.FIXED64:
                writer.fixed64(value);
                break;
            case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.BYTES:
                writer.bytes(value);
                break;
            case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.FIXED32:
                writer.fixed32(value);
                break;
            case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.SFIXED32:
                writer.sfixed32(value);
                break;
            case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.SFIXED64:
                writer.sfixed64(value);
                break;
            case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.SINT64:
                writer.sint64(value);
                break;
            case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.UINT32:
                writer.uint32(value);
                break;
            case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.SINT32:
                writer.sint32(value);
                break;
        }
    }
    catch (e) {
        if (e instanceof Error) {
            throw new Error(`cannot encode field ${msgName}.${fieldName} to binary: ${e.message}`);
        }
        throw e;
    }
}
function writeTypeOfScalar(type) {
    switch (type) {
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.BYTES:
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.STRING:
            return _wire_binary_encoding_js__WEBPACK_IMPORTED_MODULE_0__/* .WireType */ .O0.LengthDelimited;
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.DOUBLE:
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.FIXED64:
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.SFIXED64:
            return _wire_binary_encoding_js__WEBPACK_IMPORTED_MODULE_0__/* .WireType */ .O0.Bit64;
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.FIXED32:
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.SFIXED32:
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.FLOAT:
            return _wire_binary_encoding_js__WEBPACK_IMPORTED_MODULE_0__/* .WireType */ .O0.Bit32;
        default:
            return _wire_binary_encoding_js__WEBPACK_IMPORTED_MODULE_0__/* .WireType */ .O0.Varint;
    }
}


/***/ }),

/***/ 8461:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  tr: () => (/* binding */ FileDescriptorProtoSchema),
  C9: () => (/* binding */ MethodOptions_IdempotencyLevel)
});

// UNUSED EXPORTS: DescriptorProtoSchema, DescriptorProto_ExtensionRangeSchema, DescriptorProto_ReservedRangeSchema, Edition, EditionSchema, EnumDescriptorProtoSchema, EnumDescriptorProto_EnumReservedRangeSchema, EnumOptionsSchema, EnumValueDescriptorProtoSchema, EnumValueOptionsSchema, ExtensionRangeOptionsSchema, ExtensionRangeOptions_DeclarationSchema, ExtensionRangeOptions_VerificationState, ExtensionRangeOptions_VerificationStateSchema, FeatureSetDefaultsSchema, FeatureSetDefaults_FeatureSetEditionDefaultSchema, FeatureSetSchema, FeatureSet_EnforceNamingStyle, FeatureSet_EnforceNamingStyleSchema, FeatureSet_EnumType, FeatureSet_EnumTypeSchema, FeatureSet_FieldPresence, FeatureSet_FieldPresenceSchema, FeatureSet_JsonFormat, FeatureSet_JsonFormatSchema, FeatureSet_MessageEncoding, FeatureSet_MessageEncodingSchema, FeatureSet_RepeatedFieldEncoding, FeatureSet_RepeatedFieldEncodingSchema, FeatureSet_Utf8Validation, FeatureSet_Utf8ValidationSchema, FeatureSet_VisibilityFeatureSchema, FeatureSet_VisibilityFeature_DefaultSymbolVisibility, FeatureSet_VisibilityFeature_DefaultSymbolVisibilitySchema, FieldDescriptorProtoSchema, FieldDescriptorProto_Label, FieldDescriptorProto_LabelSchema, FieldDescriptorProto_Type, FieldDescriptorProto_TypeSchema, FieldOptionsSchema, FieldOptions_CType, FieldOptions_CTypeSchema, FieldOptions_EditionDefaultSchema, FieldOptions_FeatureSupportSchema, FieldOptions_JSType, FieldOptions_JSTypeSchema, FieldOptions_OptionRetention, FieldOptions_OptionRetentionSchema, FieldOptions_OptionTargetType, FieldOptions_OptionTargetTypeSchema, FileDescriptorSetSchema, FileOptionsSchema, FileOptions_OptimizeMode, FileOptions_OptimizeModeSchema, GeneratedCodeInfoSchema, GeneratedCodeInfo_AnnotationSchema, GeneratedCodeInfo_Annotation_Semantic, GeneratedCodeInfo_Annotation_SemanticSchema, MessageOptionsSchema, MethodDescriptorProtoSchema, MethodOptionsSchema, MethodOptions_IdempotencyLevelSchema, OneofDescriptorProtoSchema, OneofOptionsSchema, ServiceDescriptorProtoSchema, ServiceOptionsSchema, SourceCodeInfoSchema, SourceCodeInfo_LocationSchema, SymbolVisibility, SymbolVisibilitySchema, UninterpretedOptionSchema, UninterpretedOption_NamePartSchema, file_google_protobuf_descriptor

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/codegenv2/restore-json-names.js
var restore_json_names = __webpack_require__(12140);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/registry.js + 2 modules
var registry = __webpack_require__(31503);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/codegenv2/boot.js
// Copyright 2021-2026 Buf Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.


/**
 * Hydrate a file descriptor for google/protobuf/descriptor.proto from a plain
 * object.
 *
 * See createFileDescriptorProtoBoot() for details.
 *
 * @private
 */
function boot(boot) {
    const root = bootFileDescriptorProto(boot);
    root.messageType.forEach(restore_json_names/* restoreJsonNames */.h);
    const reg = (0,registry/* createFileRegistry */.uO)(root, () => undefined);
    // biome-ignore lint/style/noNonNullAssertion: non-null assertion because we just created the registry from the file we look up
    return reg.getFile(root.name);
}
/**
 * Creates the message google.protobuf.FileDescriptorProto from an object literal.
 *
 * See createFileDescriptorProtoBoot() for details.
 *
 * @private
 */
function bootFileDescriptorProto(init) {
    const proto = Object.create({
        syntax: "",
        edition: 0,
    });
    return Object.assign(proto, Object.assign(Object.assign({ $typeName: "google.protobuf.FileDescriptorProto", dependency: [], publicDependency: [], weakDependency: [], optionDependency: [], service: [], extension: [] }, init), { messageType: init.messageType.map(bootDescriptorProto), enumType: init.enumType.map(bootEnumDescriptorProto) }));
}
function bootDescriptorProto(init) {
    var _a, _b, _c, _d, _e, _f, _g, _h;
    const proto = Object.create({
        visibility: 0,
    });
    return Object.assign(proto, {
        $typeName: "google.protobuf.DescriptorProto",
        name: init.name,
        field: (_b = (_a = init.field) === null || _a === void 0 ? void 0 : _a.map(bootFieldDescriptorProto)) !== null && _b !== void 0 ? _b : [],
        extension: [],
        nestedType: (_d = (_c = init.nestedType) === null || _c === void 0 ? void 0 : _c.map(bootDescriptorProto)) !== null && _d !== void 0 ? _d : [],
        enumType: (_f = (_e = init.enumType) === null || _e === void 0 ? void 0 : _e.map(bootEnumDescriptorProto)) !== null && _f !== void 0 ? _f : [],
        extensionRange: (_h = (_g = init.extensionRange) === null || _g === void 0 ? void 0 : _g.map((e) => (Object.assign({ $typeName: "google.protobuf.DescriptorProto.ExtensionRange" }, e)))) !== null && _h !== void 0 ? _h : [],
        oneofDecl: [],
        reservedRange: [],
        reservedName: [],
    });
}
function bootFieldDescriptorProto(init) {
    const proto = Object.create({
        label: 1,
        typeName: "",
        extendee: "",
        defaultValue: "",
        oneofIndex: 0,
        jsonName: "",
        proto3Optional: false,
    });
    return Object.assign(proto, Object.assign(Object.assign({ $typeName: "google.protobuf.FieldDescriptorProto" }, init), { options: init.options ? bootFieldOptions(init.options) : undefined }));
}
function bootFieldOptions(init) {
    var _a, _b, _c;
    const proto = Object.create({
        ctype: 0,
        packed: false,
        jstype: 0,
        lazy: false,
        unverifiedLazy: false,
        deprecated: false,
        weak: false,
        debugRedact: false,
        retention: 0,
    });
    return Object.assign(proto, Object.assign(Object.assign({ $typeName: "google.protobuf.FieldOptions" }, init), { targets: (_a = init.targets) !== null && _a !== void 0 ? _a : [], editionDefaults: (_c = (_b = init.editionDefaults) === null || _b === void 0 ? void 0 : _b.map((e) => (Object.assign({ $typeName: "google.protobuf.FieldOptions.EditionDefault" }, e)))) !== null && _c !== void 0 ? _c : [], uninterpretedOption: [] }));
}
function bootEnumDescriptorProto(init) {
    const proto = Object.create({
        visibility: 0,
    });
    return Object.assign(proto, {
        $typeName: "google.protobuf.EnumDescriptorProto",
        name: init.name,
        reservedName: [],
        reservedRange: [],
        value: init.value.map((e) => (Object.assign({ $typeName: "google.protobuf.EnumValueDescriptorProto" }, e))),
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/codegenv2/message.js
var message = __webpack_require__(15033);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/wkt/gen/google/protobuf/descriptor_pb.js
// Copyright 2021-2026 Buf Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.



/**
 * Describes the file google/protobuf/descriptor.proto.
 */
const file_google_protobuf_descriptor = /*@__PURE__*/ boot({ "name": "google/protobuf/descriptor.proto", "package": "google.protobuf", "messageType": [{ "name": "FileDescriptorSet", "field": [{ "name": "file", "number": 1, "type": 11, "label": 3, "typeName": ".google.protobuf.FileDescriptorProto" }], "extensionRange": [{ "start": 536000000, "end": 536000001 }] }, { "name": "FileDescriptorProto", "field": [{ "name": "name", "number": 1, "type": 9, "label": 1 }, { "name": "package", "number": 2, "type": 9, "label": 1 }, { "name": "dependency", "number": 3, "type": 9, "label": 3 }, { "name": "public_dependency", "number": 10, "type": 5, "label": 3 }, { "name": "weak_dependency", "number": 11, "type": 5, "label": 3 }, { "name": "option_dependency", "number": 15, "type": 9, "label": 3 }, { "name": "message_type", "number": 4, "type": 11, "label": 3, "typeName": ".google.protobuf.DescriptorProto" }, { "name": "enum_type", "number": 5, "type": 11, "label": 3, "typeName": ".google.protobuf.EnumDescriptorProto" }, { "name": "service", "number": 6, "type": 11, "label": 3, "typeName": ".google.protobuf.ServiceDescriptorProto" }, { "name": "extension", "number": 7, "type": 11, "label": 3, "typeName": ".google.protobuf.FieldDescriptorProto" }, { "name": "options", "number": 8, "type": 11, "label": 1, "typeName": ".google.protobuf.FileOptions" }, { "name": "source_code_info", "number": 9, "type": 11, "label": 1, "typeName": ".google.protobuf.SourceCodeInfo" }, { "name": "syntax", "number": 12, "type": 9, "label": 1 }, { "name": "edition", "number": 14, "type": 14, "label": 1, "typeName": ".google.protobuf.Edition" }] }, { "name": "DescriptorProto", "field": [{ "name": "name", "number": 1, "type": 9, "label": 1 }, { "name": "field", "number": 2, "type": 11, "label": 3, "typeName": ".google.protobuf.FieldDescriptorProto" }, { "name": "extension", "number": 6, "type": 11, "label": 3, "typeName": ".google.protobuf.FieldDescriptorProto" }, { "name": "nested_type", "number": 3, "type": 11, "label": 3, "typeName": ".google.protobuf.DescriptorProto" }, { "name": "enum_type", "number": 4, "type": 11, "label": 3, "typeName": ".google.protobuf.EnumDescriptorProto" }, { "name": "extension_range", "number": 5, "type": 11, "label": 3, "typeName": ".google.protobuf.DescriptorProto.ExtensionRange" }, { "name": "oneof_decl", "number": 8, "type": 11, "label": 3, "typeName": ".google.protobuf.OneofDescriptorProto" }, { "name": "options", "number": 7, "type": 11, "label": 1, "typeName": ".google.protobuf.MessageOptions" }, { "name": "reserved_range", "number": 9, "type": 11, "label": 3, "typeName": ".google.protobuf.DescriptorProto.ReservedRange" }, { "name": "reserved_name", "number": 10, "type": 9, "label": 3 }, { "name": "visibility", "number": 11, "type": 14, "label": 1, "typeName": ".google.protobuf.SymbolVisibility" }], "nestedType": [{ "name": "ExtensionRange", "field": [{ "name": "start", "number": 1, "type": 5, "label": 1 }, { "name": "end", "number": 2, "type": 5, "label": 1 }, { "name": "options", "number": 3, "type": 11, "label": 1, "typeName": ".google.protobuf.ExtensionRangeOptions" }] }, { "name": "ReservedRange", "field": [{ "name": "start", "number": 1, "type": 5, "label": 1 }, { "name": "end", "number": 2, "type": 5, "label": 1 }] }] }, { "name": "ExtensionRangeOptions", "field": [{ "name": "uninterpreted_option", "number": 999, "type": 11, "label": 3, "typeName": ".google.protobuf.UninterpretedOption" }, { "name": "declaration", "number": 2, "type": 11, "label": 3, "typeName": ".google.protobuf.ExtensionRangeOptions.Declaration", "options": { "retention": 2 } }, { "name": "features", "number": 50, "type": 11, "label": 1, "typeName": ".google.protobuf.FeatureSet" }, { "name": "verification", "number": 3, "type": 14, "label": 1, "typeName": ".google.protobuf.ExtensionRangeOptions.VerificationState", "defaultValue": "UNVERIFIED", "options": { "retention": 2 } }], "nestedType": [{ "name": "Declaration", "field": [{ "name": "number", "number": 1, "type": 5, "label": 1 }, { "name": "full_name", "number": 2, "type": 9, "label": 1 }, { "name": "type", "number": 3, "type": 9, "label": 1 }, { "name": "reserved", "number": 5, "type": 8, "label": 1 }, { "name": "repeated", "number": 6, "type": 8, "label": 1 }] }], "enumType": [{ "name": "VerificationState", "value": [{ "name": "DECLARATION", "number": 0 }, { "name": "UNVERIFIED", "number": 1 }] }], "extensionRange": [{ "start": 1000, "end": 536870912 }] }, { "name": "FieldDescriptorProto", "field": [{ "name": "name", "number": 1, "type": 9, "label": 1 }, { "name": "number", "number": 3, "type": 5, "label": 1 }, { "name": "label", "number": 4, "type": 14, "label": 1, "typeName": ".google.protobuf.FieldDescriptorProto.Label" }, { "name": "type", "number": 5, "type": 14, "label": 1, "typeName": ".google.protobuf.FieldDescriptorProto.Type" }, { "name": "type_name", "number": 6, "type": 9, "label": 1 }, { "name": "extendee", "number": 2, "type": 9, "label": 1 }, { "name": "default_value", "number": 7, "type": 9, "label": 1 }, { "name": "oneof_index", "number": 9, "type": 5, "label": 1 }, { "name": "json_name", "number": 10, "type": 9, "label": 1 }, { "name": "options", "number": 8, "type": 11, "label": 1, "typeName": ".google.protobuf.FieldOptions" }, { "name": "proto3_optional", "number": 17, "type": 8, "label": 1 }], "enumType": [{ "name": "Type", "value": [{ "name": "TYPE_DOUBLE", "number": 1 }, { "name": "TYPE_FLOAT", "number": 2 }, { "name": "TYPE_INT64", "number": 3 }, { "name": "TYPE_UINT64", "number": 4 }, { "name": "TYPE_INT32", "number": 5 }, { "name": "TYPE_FIXED64", "number": 6 }, { "name": "TYPE_FIXED32", "number": 7 }, { "name": "TYPE_BOOL", "number": 8 }, { "name": "TYPE_STRING", "number": 9 }, { "name": "TYPE_GROUP", "number": 10 }, { "name": "TYPE_MESSAGE", "number": 11 }, { "name": "TYPE_BYTES", "number": 12 }, { "name": "TYPE_UINT32", "number": 13 }, { "name": "TYPE_ENUM", "number": 14 }, { "name": "TYPE_SFIXED32", "number": 15 }, { "name": "TYPE_SFIXED64", "number": 16 }, { "name": "TYPE_SINT32", "number": 17 }, { "name": "TYPE_SINT64", "number": 18 }] }, { "name": "Label", "value": [{ "name": "LABEL_OPTIONAL", "number": 1 }, { "name": "LABEL_REPEATED", "number": 3 }, { "name": "LABEL_REQUIRED", "number": 2 }] }] }, { "name": "OneofDescriptorProto", "field": [{ "name": "name", "number": 1, "type": 9, "label": 1 }, { "name": "options", "number": 2, "type": 11, "label": 1, "typeName": ".google.protobuf.OneofOptions" }] }, { "name": "EnumDescriptorProto", "field": [{ "name": "name", "number": 1, "type": 9, "label": 1 }, { "name": "value", "number": 2, "type": 11, "label": 3, "typeName": ".google.protobuf.EnumValueDescriptorProto" }, { "name": "options", "number": 3, "type": 11, "label": 1, "typeName": ".google.protobuf.EnumOptions" }, { "name": "reserved_range", "number": 4, "type": 11, "label": 3, "typeName": ".google.protobuf.EnumDescriptorProto.EnumReservedRange" }, { "name": "reserved_name", "number": 5, "type": 9, "label": 3 }, { "name": "visibility", "number": 6, "type": 14, "label": 1, "typeName": ".google.protobuf.SymbolVisibility" }], "nestedType": [{ "name": "EnumReservedRange", "field": [{ "name": "start", "number": 1, "type": 5, "label": 1 }, { "name": "end", "number": 2, "type": 5, "label": 1 }] }] }, { "name": "EnumValueDescriptorProto", "field": [{ "name": "name", "number": 1, "type": 9, "label": 1 }, { "name": "number", "number": 2, "type": 5, "label": 1 }, { "name": "options", "number": 3, "type": 11, "label": 1, "typeName": ".google.protobuf.EnumValueOptions" }] }, { "name": "ServiceDescriptorProto", "field": [{ "name": "name", "number": 1, "type": 9, "label": 1 }, { "name": "method", "number": 2, "type": 11, "label": 3, "typeName": ".google.protobuf.MethodDescriptorProto" }, { "name": "options", "number": 3, "type": 11, "label": 1, "typeName": ".google.protobuf.ServiceOptions" }] }, { "name": "MethodDescriptorProto", "field": [{ "name": "name", "number": 1, "type": 9, "label": 1 }, { "name": "input_type", "number": 2, "type": 9, "label": 1 }, { "name": "output_type", "number": 3, "type": 9, "label": 1 }, { "name": "options", "number": 4, "type": 11, "label": 1, "typeName": ".google.protobuf.MethodOptions" }, { "name": "client_streaming", "number": 5, "type": 8, "label": 1, "defaultValue": "false" }, { "name": "server_streaming", "number": 6, "type": 8, "label": 1, "defaultValue": "false" }] }, { "name": "FileOptions", "field": [{ "name": "java_package", "number": 1, "type": 9, "label": 1 }, { "name": "java_outer_classname", "number": 8, "type": 9, "label": 1 }, { "name": "java_multiple_files", "number": 10, "type": 8, "label": 1, "defaultValue": "false" }, { "name": "java_generate_equals_and_hash", "number": 20, "type": 8, "label": 1, "options": { "deprecated": true } }, { "name": "java_string_check_utf8", "number": 27, "type": 8, "label": 1, "defaultValue": "false" }, { "name": "optimize_for", "number": 9, "type": 14, "label": 1, "typeName": ".google.protobuf.FileOptions.OptimizeMode", "defaultValue": "SPEED" }, { "name": "go_package", "number": 11, "type": 9, "label": 1 }, { "name": "cc_generic_services", "number": 16, "type": 8, "label": 1, "defaultValue": "false" }, { "name": "java_generic_services", "number": 17, "type": 8, "label": 1, "defaultValue": "false" }, { "name": "py_generic_services", "number": 18, "type": 8, "label": 1, "defaultValue": "false" }, { "name": "deprecated", "number": 23, "type": 8, "label": 1, "defaultValue": "false" }, { "name": "cc_enable_arenas", "number": 31, "type": 8, "label": 1, "defaultValue": "true" }, { "name": "objc_class_prefix", "number": 36, "type": 9, "label": 1 }, { "name": "csharp_namespace", "number": 37, "type": 9, "label": 1 }, { "name": "swift_prefix", "number": 39, "type": 9, "label": 1 }, { "name": "php_class_prefix", "number": 40, "type": 9, "label": 1 }, { "name": "php_namespace", "number": 41, "type": 9, "label": 1 }, { "name": "php_metadata_namespace", "number": 44, "type": 9, "label": 1 }, { "name": "ruby_package", "number": 45, "type": 9, "label": 1 }, { "name": "features", "number": 50, "type": 11, "label": 1, "typeName": ".google.protobuf.FeatureSet" }, { "name": "uninterpreted_option", "number": 999, "type": 11, "label": 3, "typeName": ".google.protobuf.UninterpretedOption" }], "enumType": [{ "name": "OptimizeMode", "value": [{ "name": "SPEED", "number": 1 }, { "name": "CODE_SIZE", "number": 2 }, { "name": "LITE_RUNTIME", "number": 3 }] }], "extensionRange": [{ "start": 1000, "end": 536870912 }] }, { "name": "MessageOptions", "field": [{ "name": "message_set_wire_format", "number": 1, "type": 8, "label": 1, "defaultValue": "false" }, { "name": "no_standard_descriptor_accessor", "number": 2, "type": 8, "label": 1, "defaultValue": "false" }, { "name": "deprecated", "number": 3, "type": 8, "label": 1, "defaultValue": "false" }, { "name": "map_entry", "number": 7, "type": 8, "label": 1 }, { "name": "deprecated_legacy_json_field_conflicts", "number": 11, "type": 8, "label": 1, "options": { "deprecated": true } }, { "name": "features", "number": 12, "type": 11, "label": 1, "typeName": ".google.protobuf.FeatureSet" }, { "name": "uninterpreted_option", "number": 999, "type": 11, "label": 3, "typeName": ".google.protobuf.UninterpretedOption" }], "extensionRange": [{ "start": 1000, "end": 536870912 }] }, { "name": "FieldOptions", "field": [{ "name": "ctype", "number": 1, "type": 14, "label": 1, "typeName": ".google.protobuf.FieldOptions.CType", "defaultValue": "STRING" }, { "name": "packed", "number": 2, "type": 8, "label": 1 }, { "name": "jstype", "number": 6, "type": 14, "label": 1, "typeName": ".google.protobuf.FieldOptions.JSType", "defaultValue": "JS_NORMAL" }, { "name": "lazy", "number": 5, "type": 8, "label": 1, "defaultValue": "false" }, { "name": "unverified_lazy", "number": 15, "type": 8, "label": 1, "defaultValue": "false" }, { "name": "deprecated", "number": 3, "type": 8, "label": 1, "defaultValue": "false" }, { "name": "weak", "number": 10, "type": 8, "label": 1, "defaultValue": "false", "options": { "deprecated": true } }, { "name": "debug_redact", "number": 16, "type": 8, "label": 1, "defaultValue": "false" }, { "name": "retention", "number": 17, "type": 14, "label": 1, "typeName": ".google.protobuf.FieldOptions.OptionRetention" }, { "name": "targets", "number": 19, "type": 14, "label": 3, "typeName": ".google.protobuf.FieldOptions.OptionTargetType" }, { "name": "edition_defaults", "number": 20, "type": 11, "label": 3, "typeName": ".google.protobuf.FieldOptions.EditionDefault" }, { "name": "features", "number": 21, "type": 11, "label": 1, "typeName": ".google.protobuf.FeatureSet" }, { "name": "feature_support", "number": 22, "type": 11, "label": 1, "typeName": ".google.protobuf.FieldOptions.FeatureSupport" }, { "name": "uninterpreted_option", "number": 999, "type": 11, "label": 3, "typeName": ".google.protobuf.UninterpretedOption" }], "nestedType": [{ "name": "EditionDefault", "field": [{ "name": "edition", "number": 3, "type": 14, "label": 1, "typeName": ".google.protobuf.Edition" }, { "name": "value", "number": 2, "type": 9, "label": 1 }] }, { "name": "FeatureSupport", "field": [{ "name": "edition_introduced", "number": 1, "type": 14, "label": 1, "typeName": ".google.protobuf.Edition" }, { "name": "edition_deprecated", "number": 2, "type": 14, "label": 1, "typeName": ".google.protobuf.Edition" }, { "name": "deprecation_warning", "number": 3, "type": 9, "label": 1 }, { "name": "edition_removed", "number": 4, "type": 14, "label": 1, "typeName": ".google.protobuf.Edition" }] }], "enumType": [{ "name": "CType", "value": [{ "name": "STRING", "number": 0 }, { "name": "CORD", "number": 1 }, { "name": "STRING_PIECE", "number": 2 }] }, { "name": "JSType", "value": [{ "name": "JS_NORMAL", "number": 0 }, { "name": "JS_STRING", "number": 1 }, { "name": "JS_NUMBER", "number": 2 }] }, { "name": "OptionRetention", "value": [{ "name": "RETENTION_UNKNOWN", "number": 0 }, { "name": "RETENTION_RUNTIME", "number": 1 }, { "name": "RETENTION_SOURCE", "number": 2 }] }, { "name": "OptionTargetType", "value": [{ "name": "TARGET_TYPE_UNKNOWN", "number": 0 }, { "name": "TARGET_TYPE_FILE", "number": 1 }, { "name": "TARGET_TYPE_EXTENSION_RANGE", "number": 2 }, { "name": "TARGET_TYPE_MESSAGE", "number": 3 }, { "name": "TARGET_TYPE_FIELD", "number": 4 }, { "name": "TARGET_TYPE_ONEOF", "number": 5 }, { "name": "TARGET_TYPE_ENUM", "number": 6 }, { "name": "TARGET_TYPE_ENUM_ENTRY", "number": 7 }, { "name": "TARGET_TYPE_SERVICE", "number": 8 }, { "name": "TARGET_TYPE_METHOD", "number": 9 }] }], "extensionRange": [{ "start": 1000, "end": 536870912 }] }, { "name": "OneofOptions", "field": [{ "name": "features", "number": 1, "type": 11, "label": 1, "typeName": ".google.protobuf.FeatureSet" }, { "name": "uninterpreted_option", "number": 999, "type": 11, "label": 3, "typeName": ".google.protobuf.UninterpretedOption" }], "extensionRange": [{ "start": 1000, "end": 536870912 }] }, { "name": "EnumOptions", "field": [{ "name": "allow_alias", "number": 2, "type": 8, "label": 1 }, { "name": "deprecated", "number": 3, "type": 8, "label": 1, "defaultValue": "false" }, { "name": "deprecated_legacy_json_field_conflicts", "number": 6, "type": 8, "label": 1, "options": { "deprecated": true } }, { "name": "features", "number": 7, "type": 11, "label": 1, "typeName": ".google.protobuf.FeatureSet" }, { "name": "uninterpreted_option", "number": 999, "type": 11, "label": 3, "typeName": ".google.protobuf.UninterpretedOption" }], "extensionRange": [{ "start": 1000, "end": 536870912 }] }, { "name": "EnumValueOptions", "field": [{ "name": "deprecated", "number": 1, "type": 8, "label": 1, "defaultValue": "false" }, { "name": "features", "number": 2, "type": 11, "label": 1, "typeName": ".google.protobuf.FeatureSet" }, { "name": "debug_redact", "number": 3, "type": 8, "label": 1, "defaultValue": "false" }, { "name": "feature_support", "number": 4, "type": 11, "label": 1, "typeName": ".google.protobuf.FieldOptions.FeatureSupport" }, { "name": "uninterpreted_option", "number": 999, "type": 11, "label": 3, "typeName": ".google.protobuf.UninterpretedOption" }], "extensionRange": [{ "start": 1000, "end": 536870912 }] }, { "name": "ServiceOptions", "field": [{ "name": "features", "number": 34, "type": 11, "label": 1, "typeName": ".google.protobuf.FeatureSet" }, { "name": "deprecated", "number": 33, "type": 8, "label": 1, "defaultValue": "false" }, { "name": "uninterpreted_option", "number": 999, "type": 11, "label": 3, "typeName": ".google.protobuf.UninterpretedOption" }], "extensionRange": [{ "start": 1000, "end": 536870912 }] }, { "name": "MethodOptions", "field": [{ "name": "deprecated", "number": 33, "type": 8, "label": 1, "defaultValue": "false" }, { "name": "idempotency_level", "number": 34, "type": 14, "label": 1, "typeName": ".google.protobuf.MethodOptions.IdempotencyLevel", "defaultValue": "IDEMPOTENCY_UNKNOWN" }, { "name": "features", "number": 35, "type": 11, "label": 1, "typeName": ".google.protobuf.FeatureSet" }, { "name": "uninterpreted_option", "number": 999, "type": 11, "label": 3, "typeName": ".google.protobuf.UninterpretedOption" }], "enumType": [{ "name": "IdempotencyLevel", "value": [{ "name": "IDEMPOTENCY_UNKNOWN", "number": 0 }, { "name": "NO_SIDE_EFFECTS", "number": 1 }, { "name": "IDEMPOTENT", "number": 2 }] }], "extensionRange": [{ "start": 1000, "end": 536870912 }] }, { "name": "UninterpretedOption", "field": [{ "name": "name", "number": 2, "type": 11, "label": 3, "typeName": ".google.protobuf.UninterpretedOption.NamePart" }, { "name": "identifier_value", "number": 3, "type": 9, "label": 1 }, { "name": "positive_int_value", "number": 4, "type": 4, "label": 1 }, { "name": "negative_int_value", "number": 5, "type": 3, "label": 1 }, { "name": "double_value", "number": 6, "type": 1, "label": 1 }, { "name": "string_value", "number": 7, "type": 12, "label": 1 }, { "name": "aggregate_value", "number": 8, "type": 9, "label": 1 }], "nestedType": [{ "name": "NamePart", "field": [{ "name": "name_part", "number": 1, "type": 9, "label": 2 }, { "name": "is_extension", "number": 2, "type": 8, "label": 2 }] }] }, { "name": "FeatureSet", "field": [{ "name": "field_presence", "number": 1, "type": 14, "label": 1, "typeName": ".google.protobuf.FeatureSet.FieldPresence", "options": { "retention": 1, "targets": [4, 1], "editionDefaults": [{ "value": "EXPLICIT", "edition": 900 }, { "value": "IMPLICIT", "edition": 999 }, { "value": "EXPLICIT", "edition": 1000 }] } }, { "name": "enum_type", "number": 2, "type": 14, "label": 1, "typeName": ".google.protobuf.FeatureSet.EnumType", "options": { "retention": 1, "targets": [6, 1], "editionDefaults": [{ "value": "CLOSED", "edition": 900 }, { "value": "OPEN", "edition": 999 }] } }, { "name": "repeated_field_encoding", "number": 3, "type": 14, "label": 1, "typeName": ".google.protobuf.FeatureSet.RepeatedFieldEncoding", "options": { "retention": 1, "targets": [4, 1], "editionDefaults": [{ "value": "EXPANDED", "edition": 900 }, { "value": "PACKED", "edition": 999 }] } }, { "name": "utf8_validation", "number": 4, "type": 14, "label": 1, "typeName": ".google.protobuf.FeatureSet.Utf8Validation", "options": { "retention": 1, "targets": [4, 1], "editionDefaults": [{ "value": "NONE", "edition": 900 }, { "value": "VERIFY", "edition": 999 }] } }, { "name": "message_encoding", "number": 5, "type": 14, "label": 1, "typeName": ".google.protobuf.FeatureSet.MessageEncoding", "options": { "retention": 1, "targets": [4, 1], "editionDefaults": [{ "value": "LENGTH_PREFIXED", "edition": 900 }] } }, { "name": "json_format", "number": 6, "type": 14, "label": 1, "typeName": ".google.protobuf.FeatureSet.JsonFormat", "options": { "retention": 1, "targets": [3, 6, 1], "editionDefaults": [{ "value": "LEGACY_BEST_EFFORT", "edition": 900 }, { "value": "ALLOW", "edition": 999 }] } }, { "name": "enforce_naming_style", "number": 7, "type": 14, "label": 1, "typeName": ".google.protobuf.FeatureSet.EnforceNamingStyle", "options": { "retention": 2, "targets": [1, 2, 3, 4, 5, 6, 7, 8, 9], "editionDefaults": [{ "value": "STYLE_LEGACY", "edition": 900 }, { "value": "STYLE2024", "edition": 1001 }] } }, { "name": "default_symbol_visibility", "number": 8, "type": 14, "label": 1, "typeName": ".google.protobuf.FeatureSet.VisibilityFeature.DefaultSymbolVisibility", "options": { "retention": 2, "targets": [1], "editionDefaults": [{ "value": "EXPORT_ALL", "edition": 900 }, { "value": "EXPORT_TOP_LEVEL", "edition": 1001 }] } }], "nestedType": [{ "name": "VisibilityFeature", "enumType": [{ "name": "DefaultSymbolVisibility", "value": [{ "name": "DEFAULT_SYMBOL_VISIBILITY_UNKNOWN", "number": 0 }, { "name": "EXPORT_ALL", "number": 1 }, { "name": "EXPORT_TOP_LEVEL", "number": 2 }, { "name": "LOCAL_ALL", "number": 3 }, { "name": "STRICT", "number": 4 }] }] }], "enumType": [{ "name": "FieldPresence", "value": [{ "name": "FIELD_PRESENCE_UNKNOWN", "number": 0 }, { "name": "EXPLICIT", "number": 1 }, { "name": "IMPLICIT", "number": 2 }, { "name": "LEGACY_REQUIRED", "number": 3 }] }, { "name": "EnumType", "value": [{ "name": "ENUM_TYPE_UNKNOWN", "number": 0 }, { "name": "OPEN", "number": 1 }, { "name": "CLOSED", "number": 2 }] }, { "name": "RepeatedFieldEncoding", "value": [{ "name": "REPEATED_FIELD_ENCODING_UNKNOWN", "number": 0 }, { "name": "PACKED", "number": 1 }, { "name": "EXPANDED", "number": 2 }] }, { "name": "Utf8Validation", "value": [{ "name": "UTF8_VALIDATION_UNKNOWN", "number": 0 }, { "name": "VERIFY", "number": 2 }, { "name": "NONE", "number": 3 }] }, { "name": "MessageEncoding", "value": [{ "name": "MESSAGE_ENCODING_UNKNOWN", "number": 0 }, { "name": "LENGTH_PREFIXED", "number": 1 }, { "name": "DELIMITED", "number": 2 }] }, { "name": "JsonFormat", "value": [{ "name": "JSON_FORMAT_UNKNOWN", "number": 0 }, { "name": "ALLOW", "number": 1 }, { "name": "LEGACY_BEST_EFFORT", "number": 2 }] }, { "name": "EnforceNamingStyle", "value": [{ "name": "ENFORCE_NAMING_STYLE_UNKNOWN", "number": 0 }, { "name": "STYLE2024", "number": 1 }, { "name": "STYLE_LEGACY", "number": 2 }] }], "extensionRange": [{ "start": 1000, "end": 9995 }, { "start": 9995, "end": 10000 }, { "start": 10000, "end": 10001 }] }, { "name": "FeatureSetDefaults", "field": [{ "name": "defaults", "number": 1, "type": 11, "label": 3, "typeName": ".google.protobuf.FeatureSetDefaults.FeatureSetEditionDefault" }, { "name": "minimum_edition", "number": 4, "type": 14, "label": 1, "typeName": ".google.protobuf.Edition" }, { "name": "maximum_edition", "number": 5, "type": 14, "label": 1, "typeName": ".google.protobuf.Edition" }], "nestedType": [{ "name": "FeatureSetEditionDefault", "field": [{ "name": "edition", "number": 3, "type": 14, "label": 1, "typeName": ".google.protobuf.Edition" }, { "name": "overridable_features", "number": 4, "type": 11, "label": 1, "typeName": ".google.protobuf.FeatureSet" }, { "name": "fixed_features", "number": 5, "type": 11, "label": 1, "typeName": ".google.protobuf.FeatureSet" }] }] }, { "name": "SourceCodeInfo", "field": [{ "name": "location", "number": 1, "type": 11, "label": 3, "typeName": ".google.protobuf.SourceCodeInfo.Location" }], "nestedType": [{ "name": "Location", "field": [{ "name": "path", "number": 1, "type": 5, "label": 3, "options": { "packed": true } }, { "name": "span", "number": 2, "type": 5, "label": 3, "options": { "packed": true } }, { "name": "leading_comments", "number": 3, "type": 9, "label": 1 }, { "name": "trailing_comments", "number": 4, "type": 9, "label": 1 }, { "name": "leading_detached_comments", "number": 6, "type": 9, "label": 3 }] }], "extensionRange": [{ "start": 536000000, "end": 536000001 }] }, { "name": "GeneratedCodeInfo", "field": [{ "name": "annotation", "number": 1, "type": 11, "label": 3, "typeName": ".google.protobuf.GeneratedCodeInfo.Annotation" }], "nestedType": [{ "name": "Annotation", "field": [{ "name": "path", "number": 1, "type": 5, "label": 3, "options": { "packed": true } }, { "name": "source_file", "number": 2, "type": 9, "label": 1 }, { "name": "begin", "number": 3, "type": 5, "label": 1 }, { "name": "end", "number": 4, "type": 5, "label": 1 }, { "name": "semantic", "number": 5, "type": 14, "label": 1, "typeName": ".google.protobuf.GeneratedCodeInfo.Annotation.Semantic" }], "enumType": [{ "name": "Semantic", "value": [{ "name": "NONE", "number": 0 }, { "name": "SET", "number": 1 }, { "name": "ALIAS", "number": 2 }] }] }] }], "enumType": [{ "name": "Edition", "value": [{ "name": "EDITION_UNKNOWN", "number": 0 }, { "name": "EDITION_LEGACY", "number": 900 }, { "name": "EDITION_PROTO2", "number": 998 }, { "name": "EDITION_PROTO3", "number": 999 }, { "name": "EDITION_2023", "number": 1000 }, { "name": "EDITION_2024", "number": 1001 }, { "name": "EDITION_UNSTABLE", "number": 9999 }, { "name": "EDITION_1_TEST_ONLY", "number": 1 }, { "name": "EDITION_2_TEST_ONLY", "number": 2 }, { "name": "EDITION_99997_TEST_ONLY", "number": 99997 }, { "name": "EDITION_99998_TEST_ONLY", "number": 99998 }, { "name": "EDITION_99999_TEST_ONLY", "number": 99999 }, { "name": "EDITION_MAX", "number": 2147483647 }] }, { "name": "SymbolVisibility", "value": [{ "name": "VISIBILITY_UNSET", "number": 0 }, { "name": "VISIBILITY_LOCAL", "number": 1 }, { "name": "VISIBILITY_EXPORT", "number": 2 }] }] });
/**
 * Describes the message google.protobuf.FileDescriptorSet.
 * Use `create(FileDescriptorSetSchema)` to create a new message.
 */
const FileDescriptorSetSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_google_protobuf_descriptor, 0)));
/**
 * Describes the message google.protobuf.FileDescriptorProto.
 * Use `create(FileDescriptorProtoSchema)` to create a new message.
 */
const FileDescriptorProtoSchema = /*@__PURE__*/ (0,message/* messageDesc */.Z)(file_google_protobuf_descriptor, 1);
/**
 * Describes the message google.protobuf.DescriptorProto.
 * Use `create(DescriptorProtoSchema)` to create a new message.
 */
const DescriptorProtoSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_google_protobuf_descriptor, 2)));
/**
 * Describes the message google.protobuf.DescriptorProto.ExtensionRange.
 * Use `create(DescriptorProto_ExtensionRangeSchema)` to create a new message.
 */
const DescriptorProto_ExtensionRangeSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_google_protobuf_descriptor, 2, 0)));
/**
 * Describes the message google.protobuf.DescriptorProto.ReservedRange.
 * Use `create(DescriptorProto_ReservedRangeSchema)` to create a new message.
 */
const DescriptorProto_ReservedRangeSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_google_protobuf_descriptor, 2, 1)));
/**
 * Describes the message google.protobuf.ExtensionRangeOptions.
 * Use `create(ExtensionRangeOptionsSchema)` to create a new message.
 */
const ExtensionRangeOptionsSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_google_protobuf_descriptor, 3)));
/**
 * Describes the message google.protobuf.ExtensionRangeOptions.Declaration.
 * Use `create(ExtensionRangeOptions_DeclarationSchema)` to create a new message.
 */
const ExtensionRangeOptions_DeclarationSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_google_protobuf_descriptor, 3, 0)));
/**
 * The verification state of the extension range.
 *
 * @generated from enum google.protobuf.ExtensionRangeOptions.VerificationState
 */
var ExtensionRangeOptions_VerificationState;
(function (ExtensionRangeOptions_VerificationState) {
    /**
     * All the extensions of the range must be declared.
     *
     * @generated from enum value: DECLARATION = 0;
     */
    ExtensionRangeOptions_VerificationState[ExtensionRangeOptions_VerificationState["DECLARATION"] = 0] = "DECLARATION";
    /**
     * @generated from enum value: UNVERIFIED = 1;
     */
    ExtensionRangeOptions_VerificationState[ExtensionRangeOptions_VerificationState["UNVERIFIED"] = 1] = "UNVERIFIED";
})(ExtensionRangeOptions_VerificationState || (ExtensionRangeOptions_VerificationState = {}));
/**
 * Describes the enum google.protobuf.ExtensionRangeOptions.VerificationState.
 */
const ExtensionRangeOptions_VerificationStateSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (enumDesc(file_google_protobuf_descriptor, 3, 0)));
/**
 * Describes the message google.protobuf.FieldDescriptorProto.
 * Use `create(FieldDescriptorProtoSchema)` to create a new message.
 */
const FieldDescriptorProtoSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_google_protobuf_descriptor, 4)));
/**
 * @generated from enum google.protobuf.FieldDescriptorProto.Type
 */
var FieldDescriptorProto_Type;
(function (FieldDescriptorProto_Type) {
    /**
     * 0 is reserved for errors.
     * Order is weird for historical reasons.
     *
     * @generated from enum value: TYPE_DOUBLE = 1;
     */
    FieldDescriptorProto_Type[FieldDescriptorProto_Type["DOUBLE"] = 1] = "DOUBLE";
    /**
     * @generated from enum value: TYPE_FLOAT = 2;
     */
    FieldDescriptorProto_Type[FieldDescriptorProto_Type["FLOAT"] = 2] = "FLOAT";
    /**
     * Not ZigZag encoded.  Negative numbers take 10 bytes.  Use TYPE_SINT64 if
     * negative values are likely.
     *
     * @generated from enum value: TYPE_INT64 = 3;
     */
    FieldDescriptorProto_Type[FieldDescriptorProto_Type["INT64"] = 3] = "INT64";
    /**
     * @generated from enum value: TYPE_UINT64 = 4;
     */
    FieldDescriptorProto_Type[FieldDescriptorProto_Type["UINT64"] = 4] = "UINT64";
    /**
     * Not ZigZag encoded.  Negative numbers take 10 bytes.  Use TYPE_SINT32 if
     * negative values are likely.
     *
     * @generated from enum value: TYPE_INT32 = 5;
     */
    FieldDescriptorProto_Type[FieldDescriptorProto_Type["INT32"] = 5] = "INT32";
    /**
     * @generated from enum value: TYPE_FIXED64 = 6;
     */
    FieldDescriptorProto_Type[FieldDescriptorProto_Type["FIXED64"] = 6] = "FIXED64";
    /**
     * @generated from enum value: TYPE_FIXED32 = 7;
     */
    FieldDescriptorProto_Type[FieldDescriptorProto_Type["FIXED32"] = 7] = "FIXED32";
    /**
     * @generated from enum value: TYPE_BOOL = 8;
     */
    FieldDescriptorProto_Type[FieldDescriptorProto_Type["BOOL"] = 8] = "BOOL";
    /**
     * @generated from enum value: TYPE_STRING = 9;
     */
    FieldDescriptorProto_Type[FieldDescriptorProto_Type["STRING"] = 9] = "STRING";
    /**
     * Tag-delimited aggregate.
     * Group type is deprecated and not supported after google.protobuf. However, Proto3
     * implementations should still be able to parse the group wire format and
     * treat group fields as unknown fields.  In Editions, the group wire format
     * can be enabled via the `message_encoding` feature.
     *
     * @generated from enum value: TYPE_GROUP = 10;
     */
    FieldDescriptorProto_Type[FieldDescriptorProto_Type["GROUP"] = 10] = "GROUP";
    /**
     * Length-delimited aggregate.
     *
     * @generated from enum value: TYPE_MESSAGE = 11;
     */
    FieldDescriptorProto_Type[FieldDescriptorProto_Type["MESSAGE"] = 11] = "MESSAGE";
    /**
     * New in version 2.
     *
     * @generated from enum value: TYPE_BYTES = 12;
     */
    FieldDescriptorProto_Type[FieldDescriptorProto_Type["BYTES"] = 12] = "BYTES";
    /**
     * @generated from enum value: TYPE_UINT32 = 13;
     */
    FieldDescriptorProto_Type[FieldDescriptorProto_Type["UINT32"] = 13] = "UINT32";
    /**
     * @generated from enum value: TYPE_ENUM = 14;
     */
    FieldDescriptorProto_Type[FieldDescriptorProto_Type["ENUM"] = 14] = "ENUM";
    /**
     * @generated from enum value: TYPE_SFIXED32 = 15;
     */
    FieldDescriptorProto_Type[FieldDescriptorProto_Type["SFIXED32"] = 15] = "SFIXED32";
    /**
     * @generated from enum value: TYPE_SFIXED64 = 16;
     */
    FieldDescriptorProto_Type[FieldDescriptorProto_Type["SFIXED64"] = 16] = "SFIXED64";
    /**
     * Uses ZigZag encoding.
     *
     * @generated from enum value: TYPE_SINT32 = 17;
     */
    FieldDescriptorProto_Type[FieldDescriptorProto_Type["SINT32"] = 17] = "SINT32";
    /**
     * Uses ZigZag encoding.
     *
     * @generated from enum value: TYPE_SINT64 = 18;
     */
    FieldDescriptorProto_Type[FieldDescriptorProto_Type["SINT64"] = 18] = "SINT64";
})(FieldDescriptorProto_Type || (FieldDescriptorProto_Type = {}));
/**
 * Describes the enum google.protobuf.FieldDescriptorProto.Type.
 */
const FieldDescriptorProto_TypeSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (enumDesc(file_google_protobuf_descriptor, 4, 0)));
/**
 * @generated from enum google.protobuf.FieldDescriptorProto.Label
 */
var FieldDescriptorProto_Label;
(function (FieldDescriptorProto_Label) {
    /**
     * 0 is reserved for errors
     *
     * @generated from enum value: LABEL_OPTIONAL = 1;
     */
    FieldDescriptorProto_Label[FieldDescriptorProto_Label["OPTIONAL"] = 1] = "OPTIONAL";
    /**
     * @generated from enum value: LABEL_REPEATED = 3;
     */
    FieldDescriptorProto_Label[FieldDescriptorProto_Label["REPEATED"] = 3] = "REPEATED";
    /**
     * The required label is only allowed in google.protobuf.  In proto3 and Editions
     * it's explicitly prohibited.  In Editions, the `field_presence` feature
     * can be used to get this behavior.
     *
     * @generated from enum value: LABEL_REQUIRED = 2;
     */
    FieldDescriptorProto_Label[FieldDescriptorProto_Label["REQUIRED"] = 2] = "REQUIRED";
})(FieldDescriptorProto_Label || (FieldDescriptorProto_Label = {}));
/**
 * Describes the enum google.protobuf.FieldDescriptorProto.Label.
 */
const FieldDescriptorProto_LabelSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (enumDesc(file_google_protobuf_descriptor, 4, 1)));
/**
 * Describes the message google.protobuf.OneofDescriptorProto.
 * Use `create(OneofDescriptorProtoSchema)` to create a new message.
 */
const OneofDescriptorProtoSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_google_protobuf_descriptor, 5)));
/**
 * Describes the message google.protobuf.EnumDescriptorProto.
 * Use `create(EnumDescriptorProtoSchema)` to create a new message.
 */
const EnumDescriptorProtoSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_google_protobuf_descriptor, 6)));
/**
 * Describes the message google.protobuf.EnumDescriptorProto.EnumReservedRange.
 * Use `create(EnumDescriptorProto_EnumReservedRangeSchema)` to create a new message.
 */
const EnumDescriptorProto_EnumReservedRangeSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_google_protobuf_descriptor, 6, 0)));
/**
 * Describes the message google.protobuf.EnumValueDescriptorProto.
 * Use `create(EnumValueDescriptorProtoSchema)` to create a new message.
 */
const EnumValueDescriptorProtoSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_google_protobuf_descriptor, 7)));
/**
 * Describes the message google.protobuf.ServiceDescriptorProto.
 * Use `create(ServiceDescriptorProtoSchema)` to create a new message.
 */
const ServiceDescriptorProtoSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_google_protobuf_descriptor, 8)));
/**
 * Describes the message google.protobuf.MethodDescriptorProto.
 * Use `create(MethodDescriptorProtoSchema)` to create a new message.
 */
const MethodDescriptorProtoSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_google_protobuf_descriptor, 9)));
/**
 * Describes the message google.protobuf.FileOptions.
 * Use `create(FileOptionsSchema)` to create a new message.
 */
const FileOptionsSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_google_protobuf_descriptor, 10)));
/**
 * Generated classes can be optimized for speed or code size.
 *
 * @generated from enum google.protobuf.FileOptions.OptimizeMode
 */
var FileOptions_OptimizeMode;
(function (FileOptions_OptimizeMode) {
    /**
     * Generate complete code for parsing, serialization,
     *
     * @generated from enum value: SPEED = 1;
     */
    FileOptions_OptimizeMode[FileOptions_OptimizeMode["SPEED"] = 1] = "SPEED";
    /**
     * etc.
     *
     * Use ReflectionOps to implement these methods.
     *
     * @generated from enum value: CODE_SIZE = 2;
     */
    FileOptions_OptimizeMode[FileOptions_OptimizeMode["CODE_SIZE"] = 2] = "CODE_SIZE";
    /**
     * Generate code using MessageLite and the lite runtime.
     *
     * @generated from enum value: LITE_RUNTIME = 3;
     */
    FileOptions_OptimizeMode[FileOptions_OptimizeMode["LITE_RUNTIME"] = 3] = "LITE_RUNTIME";
})(FileOptions_OptimizeMode || (FileOptions_OptimizeMode = {}));
/**
 * Describes the enum google.protobuf.FileOptions.OptimizeMode.
 */
const FileOptions_OptimizeModeSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (enumDesc(file_google_protobuf_descriptor, 10, 0)));
/**
 * Describes the message google.protobuf.MessageOptions.
 * Use `create(MessageOptionsSchema)` to create a new message.
 */
const MessageOptionsSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_google_protobuf_descriptor, 11)));
/**
 * Describes the message google.protobuf.FieldOptions.
 * Use `create(FieldOptionsSchema)` to create a new message.
 */
const FieldOptionsSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_google_protobuf_descriptor, 12)));
/**
 * Describes the message google.protobuf.FieldOptions.EditionDefault.
 * Use `create(FieldOptions_EditionDefaultSchema)` to create a new message.
 */
const FieldOptions_EditionDefaultSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_google_protobuf_descriptor, 12, 0)));
/**
 * Describes the message google.protobuf.FieldOptions.FeatureSupport.
 * Use `create(FieldOptions_FeatureSupportSchema)` to create a new message.
 */
const FieldOptions_FeatureSupportSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_google_protobuf_descriptor, 12, 1)));
/**
 * @generated from enum google.protobuf.FieldOptions.CType
 */
var FieldOptions_CType;
(function (FieldOptions_CType) {
    /**
     * Default mode.
     *
     * @generated from enum value: STRING = 0;
     */
    FieldOptions_CType[FieldOptions_CType["STRING"] = 0] = "STRING";
    /**
     * The option [ctype=CORD] may be applied to a non-repeated field of type
     * "bytes". It indicates that in C++, the data should be stored in a Cord
     * instead of a string.  For very large strings, this may reduce memory
     * fragmentation. It may also allow better performance when parsing from a
     * Cord, or when parsing with aliasing enabled, as the parsed Cord may then
     * alias the original buffer.
     *
     * @generated from enum value: CORD = 1;
     */
    FieldOptions_CType[FieldOptions_CType["CORD"] = 1] = "CORD";
    /**
     * @generated from enum value: STRING_PIECE = 2;
     */
    FieldOptions_CType[FieldOptions_CType["STRING_PIECE"] = 2] = "STRING_PIECE";
})(FieldOptions_CType || (FieldOptions_CType = {}));
/**
 * Describes the enum google.protobuf.FieldOptions.CType.
 */
const FieldOptions_CTypeSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (enumDesc(file_google_protobuf_descriptor, 12, 0)));
/**
 * @generated from enum google.protobuf.FieldOptions.JSType
 */
var FieldOptions_JSType;
(function (FieldOptions_JSType) {
    /**
     * Use the default type.
     *
     * @generated from enum value: JS_NORMAL = 0;
     */
    FieldOptions_JSType[FieldOptions_JSType["JS_NORMAL"] = 0] = "JS_NORMAL";
    /**
     * Use JavaScript strings.
     *
     * @generated from enum value: JS_STRING = 1;
     */
    FieldOptions_JSType[FieldOptions_JSType["JS_STRING"] = 1] = "JS_STRING";
    /**
     * Use JavaScript numbers.
     *
     * @generated from enum value: JS_NUMBER = 2;
     */
    FieldOptions_JSType[FieldOptions_JSType["JS_NUMBER"] = 2] = "JS_NUMBER";
})(FieldOptions_JSType || (FieldOptions_JSType = {}));
/**
 * Describes the enum google.protobuf.FieldOptions.JSType.
 */
const FieldOptions_JSTypeSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (enumDesc(file_google_protobuf_descriptor, 12, 1)));
/**
 * If set to RETENTION_SOURCE, the option will be omitted from the binary.
 *
 * @generated from enum google.protobuf.FieldOptions.OptionRetention
 */
var FieldOptions_OptionRetention;
(function (FieldOptions_OptionRetention) {
    /**
     * @generated from enum value: RETENTION_UNKNOWN = 0;
     */
    FieldOptions_OptionRetention[FieldOptions_OptionRetention["RETENTION_UNKNOWN"] = 0] = "RETENTION_UNKNOWN";
    /**
     * @generated from enum value: RETENTION_RUNTIME = 1;
     */
    FieldOptions_OptionRetention[FieldOptions_OptionRetention["RETENTION_RUNTIME"] = 1] = "RETENTION_RUNTIME";
    /**
     * @generated from enum value: RETENTION_SOURCE = 2;
     */
    FieldOptions_OptionRetention[FieldOptions_OptionRetention["RETENTION_SOURCE"] = 2] = "RETENTION_SOURCE";
})(FieldOptions_OptionRetention || (FieldOptions_OptionRetention = {}));
/**
 * Describes the enum google.protobuf.FieldOptions.OptionRetention.
 */
const FieldOptions_OptionRetentionSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (enumDesc(file_google_protobuf_descriptor, 12, 2)));
/**
 * This indicates the types of entities that the field may apply to when used
 * as an option. If it is unset, then the field may be freely used as an
 * option on any kind of entity.
 *
 * @generated from enum google.protobuf.FieldOptions.OptionTargetType
 */
var FieldOptions_OptionTargetType;
(function (FieldOptions_OptionTargetType) {
    /**
     * @generated from enum value: TARGET_TYPE_UNKNOWN = 0;
     */
    FieldOptions_OptionTargetType[FieldOptions_OptionTargetType["TARGET_TYPE_UNKNOWN"] = 0] = "TARGET_TYPE_UNKNOWN";
    /**
     * @generated from enum value: TARGET_TYPE_FILE = 1;
     */
    FieldOptions_OptionTargetType[FieldOptions_OptionTargetType["TARGET_TYPE_FILE"] = 1] = "TARGET_TYPE_FILE";
    /**
     * @generated from enum value: TARGET_TYPE_EXTENSION_RANGE = 2;
     */
    FieldOptions_OptionTargetType[FieldOptions_OptionTargetType["TARGET_TYPE_EXTENSION_RANGE"] = 2] = "TARGET_TYPE_EXTENSION_RANGE";
    /**
     * @generated from enum value: TARGET_TYPE_MESSAGE = 3;
     */
    FieldOptions_OptionTargetType[FieldOptions_OptionTargetType["TARGET_TYPE_MESSAGE"] = 3] = "TARGET_TYPE_MESSAGE";
    /**
     * @generated from enum value: TARGET_TYPE_FIELD = 4;
     */
    FieldOptions_OptionTargetType[FieldOptions_OptionTargetType["TARGET_TYPE_FIELD"] = 4] = "TARGET_TYPE_FIELD";
    /**
     * @generated from enum value: TARGET_TYPE_ONEOF = 5;
     */
    FieldOptions_OptionTargetType[FieldOptions_OptionTargetType["TARGET_TYPE_ONEOF"] = 5] = "TARGET_TYPE_ONEOF";
    /**
     * @generated from enum value: TARGET_TYPE_ENUM = 6;
     */
    FieldOptions_OptionTargetType[FieldOptions_OptionTargetType["TARGET_TYPE_ENUM"] = 6] = "TARGET_TYPE_ENUM";
    /**
     * @generated from enum value: TARGET_TYPE_ENUM_ENTRY = 7;
     */
    FieldOptions_OptionTargetType[FieldOptions_OptionTargetType["TARGET_TYPE_ENUM_ENTRY"] = 7] = "TARGET_TYPE_ENUM_ENTRY";
    /**
     * @generated from enum value: TARGET_TYPE_SERVICE = 8;
     */
    FieldOptions_OptionTargetType[FieldOptions_OptionTargetType["TARGET_TYPE_SERVICE"] = 8] = "TARGET_TYPE_SERVICE";
    /**
     * @generated from enum value: TARGET_TYPE_METHOD = 9;
     */
    FieldOptions_OptionTargetType[FieldOptions_OptionTargetType["TARGET_TYPE_METHOD"] = 9] = "TARGET_TYPE_METHOD";
})(FieldOptions_OptionTargetType || (FieldOptions_OptionTargetType = {}));
/**
 * Describes the enum google.protobuf.FieldOptions.OptionTargetType.
 */
const FieldOptions_OptionTargetTypeSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (enumDesc(file_google_protobuf_descriptor, 12, 3)));
/**
 * Describes the message google.protobuf.OneofOptions.
 * Use `create(OneofOptionsSchema)` to create a new message.
 */
const OneofOptionsSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_google_protobuf_descriptor, 13)));
/**
 * Describes the message google.protobuf.EnumOptions.
 * Use `create(EnumOptionsSchema)` to create a new message.
 */
const EnumOptionsSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_google_protobuf_descriptor, 14)));
/**
 * Describes the message google.protobuf.EnumValueOptions.
 * Use `create(EnumValueOptionsSchema)` to create a new message.
 */
const EnumValueOptionsSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_google_protobuf_descriptor, 15)));
/**
 * Describes the message google.protobuf.ServiceOptions.
 * Use `create(ServiceOptionsSchema)` to create a new message.
 */
const ServiceOptionsSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_google_protobuf_descriptor, 16)));
/**
 * Describes the message google.protobuf.MethodOptions.
 * Use `create(MethodOptionsSchema)` to create a new message.
 */
const MethodOptionsSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_google_protobuf_descriptor, 17)));
/**
 * Is this method side-effect-free (or safe in HTTP parlance), or idempotent,
 * or neither? HTTP based RPC implementation may choose GET verb for safe
 * methods, and PUT verb for idempotent methods instead of the default POST.
 *
 * @generated from enum google.protobuf.MethodOptions.IdempotencyLevel
 */
var MethodOptions_IdempotencyLevel;
(function (MethodOptions_IdempotencyLevel) {
    /**
     * @generated from enum value: IDEMPOTENCY_UNKNOWN = 0;
     */
    MethodOptions_IdempotencyLevel[MethodOptions_IdempotencyLevel["IDEMPOTENCY_UNKNOWN"] = 0] = "IDEMPOTENCY_UNKNOWN";
    /**
     * implies idempotent
     *
     * @generated from enum value: NO_SIDE_EFFECTS = 1;
     */
    MethodOptions_IdempotencyLevel[MethodOptions_IdempotencyLevel["NO_SIDE_EFFECTS"] = 1] = "NO_SIDE_EFFECTS";
    /**
     * idempotent, but may have side effects
     *
     * @generated from enum value: IDEMPOTENT = 2;
     */
    MethodOptions_IdempotencyLevel[MethodOptions_IdempotencyLevel["IDEMPOTENT"] = 2] = "IDEMPOTENT";
})(MethodOptions_IdempotencyLevel || (MethodOptions_IdempotencyLevel = {}));
/**
 * Describes the enum google.protobuf.MethodOptions.IdempotencyLevel.
 */
const MethodOptions_IdempotencyLevelSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (enumDesc(file_google_protobuf_descriptor, 17, 0)));
/**
 * Describes the message google.protobuf.UninterpretedOption.
 * Use `create(UninterpretedOptionSchema)` to create a new message.
 */
const UninterpretedOptionSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_google_protobuf_descriptor, 18)));
/**
 * Describes the message google.protobuf.UninterpretedOption.NamePart.
 * Use `create(UninterpretedOption_NamePartSchema)` to create a new message.
 */
const UninterpretedOption_NamePartSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_google_protobuf_descriptor, 18, 0)));
/**
 * Describes the message google.protobuf.FeatureSet.
 * Use `create(FeatureSetSchema)` to create a new message.
 */
const FeatureSetSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_google_protobuf_descriptor, 19)));
/**
 * Describes the message google.protobuf.FeatureSet.VisibilityFeature.
 * Use `create(FeatureSet_VisibilityFeatureSchema)` to create a new message.
 */
const FeatureSet_VisibilityFeatureSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_google_protobuf_descriptor, 19, 0)));
/**
 * @generated from enum google.protobuf.FeatureSet.VisibilityFeature.DefaultSymbolVisibility
 */
var FeatureSet_VisibilityFeature_DefaultSymbolVisibility;
(function (FeatureSet_VisibilityFeature_DefaultSymbolVisibility) {
    /**
     * @generated from enum value: DEFAULT_SYMBOL_VISIBILITY_UNKNOWN = 0;
     */
    FeatureSet_VisibilityFeature_DefaultSymbolVisibility[FeatureSet_VisibilityFeature_DefaultSymbolVisibility["DEFAULT_SYMBOL_VISIBILITY_UNKNOWN"] = 0] = "DEFAULT_SYMBOL_VISIBILITY_UNKNOWN";
    /**
     * Default pre-EDITION_2024, all UNSET visibility are export.
     *
     * @generated from enum value: EXPORT_ALL = 1;
     */
    FeatureSet_VisibilityFeature_DefaultSymbolVisibility[FeatureSet_VisibilityFeature_DefaultSymbolVisibility["EXPORT_ALL"] = 1] = "EXPORT_ALL";
    /**
     * All top-level symbols default to export, nested default to local.
     *
     * @generated from enum value: EXPORT_TOP_LEVEL = 2;
     */
    FeatureSet_VisibilityFeature_DefaultSymbolVisibility[FeatureSet_VisibilityFeature_DefaultSymbolVisibility["EXPORT_TOP_LEVEL"] = 2] = "EXPORT_TOP_LEVEL";
    /**
     * All symbols default to local.
     *
     * @generated from enum value: LOCAL_ALL = 3;
     */
    FeatureSet_VisibilityFeature_DefaultSymbolVisibility[FeatureSet_VisibilityFeature_DefaultSymbolVisibility["LOCAL_ALL"] = 3] = "LOCAL_ALL";
    /**
     * All symbols local by default. Nested types cannot be exported.
     * With special case caveat for message { enum {} reserved 1 to max; }
     * This is the recommended setting for new protos.
     *
     * @generated from enum value: STRICT = 4;
     */
    FeatureSet_VisibilityFeature_DefaultSymbolVisibility[FeatureSet_VisibilityFeature_DefaultSymbolVisibility["STRICT"] = 4] = "STRICT";
})(FeatureSet_VisibilityFeature_DefaultSymbolVisibility || (FeatureSet_VisibilityFeature_DefaultSymbolVisibility = {}));
/**
 * Describes the enum google.protobuf.FeatureSet.VisibilityFeature.DefaultSymbolVisibility.
 */
const FeatureSet_VisibilityFeature_DefaultSymbolVisibilitySchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (enumDesc(file_google_protobuf_descriptor, 19, 0, 0)));
/**
 * @generated from enum google.protobuf.FeatureSet.FieldPresence
 */
var FeatureSet_FieldPresence;
(function (FeatureSet_FieldPresence) {
    /**
     * @generated from enum value: FIELD_PRESENCE_UNKNOWN = 0;
     */
    FeatureSet_FieldPresence[FeatureSet_FieldPresence["FIELD_PRESENCE_UNKNOWN"] = 0] = "FIELD_PRESENCE_UNKNOWN";
    /**
     * @generated from enum value: EXPLICIT = 1;
     */
    FeatureSet_FieldPresence[FeatureSet_FieldPresence["EXPLICIT"] = 1] = "EXPLICIT";
    /**
     * @generated from enum value: IMPLICIT = 2;
     */
    FeatureSet_FieldPresence[FeatureSet_FieldPresence["IMPLICIT"] = 2] = "IMPLICIT";
    /**
     * @generated from enum value: LEGACY_REQUIRED = 3;
     */
    FeatureSet_FieldPresence[FeatureSet_FieldPresence["LEGACY_REQUIRED"] = 3] = "LEGACY_REQUIRED";
})(FeatureSet_FieldPresence || (FeatureSet_FieldPresence = {}));
/**
 * Describes the enum google.protobuf.FeatureSet.FieldPresence.
 */
const FeatureSet_FieldPresenceSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (enumDesc(file_google_protobuf_descriptor, 19, 0)));
/**
 * @generated from enum google.protobuf.FeatureSet.EnumType
 */
var FeatureSet_EnumType;
(function (FeatureSet_EnumType) {
    /**
     * @generated from enum value: ENUM_TYPE_UNKNOWN = 0;
     */
    FeatureSet_EnumType[FeatureSet_EnumType["ENUM_TYPE_UNKNOWN"] = 0] = "ENUM_TYPE_UNKNOWN";
    /**
     * @generated from enum value: OPEN = 1;
     */
    FeatureSet_EnumType[FeatureSet_EnumType["OPEN"] = 1] = "OPEN";
    /**
     * @generated from enum value: CLOSED = 2;
     */
    FeatureSet_EnumType[FeatureSet_EnumType["CLOSED"] = 2] = "CLOSED";
})(FeatureSet_EnumType || (FeatureSet_EnumType = {}));
/**
 * Describes the enum google.protobuf.FeatureSet.EnumType.
 */
const FeatureSet_EnumTypeSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (enumDesc(file_google_protobuf_descriptor, 19, 1)));
/**
 * @generated from enum google.protobuf.FeatureSet.RepeatedFieldEncoding
 */
var FeatureSet_RepeatedFieldEncoding;
(function (FeatureSet_RepeatedFieldEncoding) {
    /**
     * @generated from enum value: REPEATED_FIELD_ENCODING_UNKNOWN = 0;
     */
    FeatureSet_RepeatedFieldEncoding[FeatureSet_RepeatedFieldEncoding["REPEATED_FIELD_ENCODING_UNKNOWN"] = 0] = "REPEATED_FIELD_ENCODING_UNKNOWN";
    /**
     * @generated from enum value: PACKED = 1;
     */
    FeatureSet_RepeatedFieldEncoding[FeatureSet_RepeatedFieldEncoding["PACKED"] = 1] = "PACKED";
    /**
     * @generated from enum value: EXPANDED = 2;
     */
    FeatureSet_RepeatedFieldEncoding[FeatureSet_RepeatedFieldEncoding["EXPANDED"] = 2] = "EXPANDED";
})(FeatureSet_RepeatedFieldEncoding || (FeatureSet_RepeatedFieldEncoding = {}));
/**
 * Describes the enum google.protobuf.FeatureSet.RepeatedFieldEncoding.
 */
const FeatureSet_RepeatedFieldEncodingSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (enumDesc(file_google_protobuf_descriptor, 19, 2)));
/**
 * @generated from enum google.protobuf.FeatureSet.Utf8Validation
 */
var FeatureSet_Utf8Validation;
(function (FeatureSet_Utf8Validation) {
    /**
     * @generated from enum value: UTF8_VALIDATION_UNKNOWN = 0;
     */
    FeatureSet_Utf8Validation[FeatureSet_Utf8Validation["UTF8_VALIDATION_UNKNOWN"] = 0] = "UTF8_VALIDATION_UNKNOWN";
    /**
     * @generated from enum value: VERIFY = 2;
     */
    FeatureSet_Utf8Validation[FeatureSet_Utf8Validation["VERIFY"] = 2] = "VERIFY";
    /**
     * @generated from enum value: NONE = 3;
     */
    FeatureSet_Utf8Validation[FeatureSet_Utf8Validation["NONE"] = 3] = "NONE";
})(FeatureSet_Utf8Validation || (FeatureSet_Utf8Validation = {}));
/**
 * Describes the enum google.protobuf.FeatureSet.Utf8Validation.
 */
const FeatureSet_Utf8ValidationSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (enumDesc(file_google_protobuf_descriptor, 19, 3)));
/**
 * @generated from enum google.protobuf.FeatureSet.MessageEncoding
 */
var FeatureSet_MessageEncoding;
(function (FeatureSet_MessageEncoding) {
    /**
     * @generated from enum value: MESSAGE_ENCODING_UNKNOWN = 0;
     */
    FeatureSet_MessageEncoding[FeatureSet_MessageEncoding["MESSAGE_ENCODING_UNKNOWN"] = 0] = "MESSAGE_ENCODING_UNKNOWN";
    /**
     * @generated from enum value: LENGTH_PREFIXED = 1;
     */
    FeatureSet_MessageEncoding[FeatureSet_MessageEncoding["LENGTH_PREFIXED"] = 1] = "LENGTH_PREFIXED";
    /**
     * @generated from enum value: DELIMITED = 2;
     */
    FeatureSet_MessageEncoding[FeatureSet_MessageEncoding["DELIMITED"] = 2] = "DELIMITED";
})(FeatureSet_MessageEncoding || (FeatureSet_MessageEncoding = {}));
/**
 * Describes the enum google.protobuf.FeatureSet.MessageEncoding.
 */
const FeatureSet_MessageEncodingSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (enumDesc(file_google_protobuf_descriptor, 19, 4)));
/**
 * @generated from enum google.protobuf.FeatureSet.JsonFormat
 */
var FeatureSet_JsonFormat;
(function (FeatureSet_JsonFormat) {
    /**
     * @generated from enum value: JSON_FORMAT_UNKNOWN = 0;
     */
    FeatureSet_JsonFormat[FeatureSet_JsonFormat["JSON_FORMAT_UNKNOWN"] = 0] = "JSON_FORMAT_UNKNOWN";
    /**
     * @generated from enum value: ALLOW = 1;
     */
    FeatureSet_JsonFormat[FeatureSet_JsonFormat["ALLOW"] = 1] = "ALLOW";
    /**
     * @generated from enum value: LEGACY_BEST_EFFORT = 2;
     */
    FeatureSet_JsonFormat[FeatureSet_JsonFormat["LEGACY_BEST_EFFORT"] = 2] = "LEGACY_BEST_EFFORT";
})(FeatureSet_JsonFormat || (FeatureSet_JsonFormat = {}));
/**
 * Describes the enum google.protobuf.FeatureSet.JsonFormat.
 */
const FeatureSet_JsonFormatSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (enumDesc(file_google_protobuf_descriptor, 19, 5)));
/**
 * @generated from enum google.protobuf.FeatureSet.EnforceNamingStyle
 */
var FeatureSet_EnforceNamingStyle;
(function (FeatureSet_EnforceNamingStyle) {
    /**
     * @generated from enum value: ENFORCE_NAMING_STYLE_UNKNOWN = 0;
     */
    FeatureSet_EnforceNamingStyle[FeatureSet_EnforceNamingStyle["ENFORCE_NAMING_STYLE_UNKNOWN"] = 0] = "ENFORCE_NAMING_STYLE_UNKNOWN";
    /**
     * @generated from enum value: STYLE2024 = 1;
     */
    FeatureSet_EnforceNamingStyle[FeatureSet_EnforceNamingStyle["STYLE2024"] = 1] = "STYLE2024";
    /**
     * @generated from enum value: STYLE_LEGACY = 2;
     */
    FeatureSet_EnforceNamingStyle[FeatureSet_EnforceNamingStyle["STYLE_LEGACY"] = 2] = "STYLE_LEGACY";
})(FeatureSet_EnforceNamingStyle || (FeatureSet_EnforceNamingStyle = {}));
/**
 * Describes the enum google.protobuf.FeatureSet.EnforceNamingStyle.
 */
const FeatureSet_EnforceNamingStyleSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (enumDesc(file_google_protobuf_descriptor, 19, 6)));
/**
 * Describes the message google.protobuf.FeatureSetDefaults.
 * Use `create(FeatureSetDefaultsSchema)` to create a new message.
 */
const FeatureSetDefaultsSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_google_protobuf_descriptor, 20)));
/**
 * Describes the message google.protobuf.FeatureSetDefaults.FeatureSetEditionDefault.
 * Use `create(FeatureSetDefaults_FeatureSetEditionDefaultSchema)` to create a new message.
 */
const FeatureSetDefaults_FeatureSetEditionDefaultSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_google_protobuf_descriptor, 20, 0)));
/**
 * Describes the message google.protobuf.SourceCodeInfo.
 * Use `create(SourceCodeInfoSchema)` to create a new message.
 */
const SourceCodeInfoSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_google_protobuf_descriptor, 21)));
/**
 * Describes the message google.protobuf.SourceCodeInfo.Location.
 * Use `create(SourceCodeInfo_LocationSchema)` to create a new message.
 */
const SourceCodeInfo_LocationSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_google_protobuf_descriptor, 21, 0)));
/**
 * Describes the message google.protobuf.GeneratedCodeInfo.
 * Use `create(GeneratedCodeInfoSchema)` to create a new message.
 */
const GeneratedCodeInfoSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_google_protobuf_descriptor, 22)));
/**
 * Describes the message google.protobuf.GeneratedCodeInfo.Annotation.
 * Use `create(GeneratedCodeInfo_AnnotationSchema)` to create a new message.
 */
const GeneratedCodeInfo_AnnotationSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (messageDesc(file_google_protobuf_descriptor, 22, 0)));
/**
 * Represents the identified object's effect on the element in the original
 * .proto file.
 *
 * @generated from enum google.protobuf.GeneratedCodeInfo.Annotation.Semantic
 */
var GeneratedCodeInfo_Annotation_Semantic;
(function (GeneratedCodeInfo_Annotation_Semantic) {
    /**
     * There is no effect or the effect is indescribable.
     *
     * @generated from enum value: NONE = 0;
     */
    GeneratedCodeInfo_Annotation_Semantic[GeneratedCodeInfo_Annotation_Semantic["NONE"] = 0] = "NONE";
    /**
     * The element is set or otherwise mutated.
     *
     * @generated from enum value: SET = 1;
     */
    GeneratedCodeInfo_Annotation_Semantic[GeneratedCodeInfo_Annotation_Semantic["SET"] = 1] = "SET";
    /**
     * An alias to the element is returned.
     *
     * @generated from enum value: ALIAS = 2;
     */
    GeneratedCodeInfo_Annotation_Semantic[GeneratedCodeInfo_Annotation_Semantic["ALIAS"] = 2] = "ALIAS";
})(GeneratedCodeInfo_Annotation_Semantic || (GeneratedCodeInfo_Annotation_Semantic = {}));
/**
 * Describes the enum google.protobuf.GeneratedCodeInfo.Annotation.Semantic.
 */
const GeneratedCodeInfo_Annotation_SemanticSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (enumDesc(file_google_protobuf_descriptor, 22, 0, 0)));
/**
 * The full set of known editions.
 *
 * @generated from enum google.protobuf.Edition
 */
var Edition;
(function (Edition) {
    /**
     * A placeholder for an unknown edition value.
     *
     * @generated from enum value: EDITION_UNKNOWN = 0;
     */
    Edition[Edition["EDITION_UNKNOWN"] = 0] = "EDITION_UNKNOWN";
    /**
     * A placeholder edition for specifying default behaviors *before* a feature
     * was first introduced.  This is effectively an "infinite past".
     *
     * @generated from enum value: EDITION_LEGACY = 900;
     */
    Edition[Edition["EDITION_LEGACY"] = 900] = "EDITION_LEGACY";
    /**
     * Legacy syntax "editions".  These pre-date editions, but behave much like
     * distinct editions.  These can't be used to specify the edition of proto
     * files, but feature definitions must supply proto2/proto3 defaults for
     * backwards compatibility.
     *
     * @generated from enum value: EDITION_PROTO2 = 998;
     */
    Edition[Edition["EDITION_PROTO2"] = 998] = "EDITION_PROTO2";
    /**
     * @generated from enum value: EDITION_PROTO3 = 999;
     */
    Edition[Edition["EDITION_PROTO3"] = 999] = "EDITION_PROTO3";
    /**
     * Editions that have been released.  The specific values are arbitrary and
     * should not be depended on, but they will always be time-ordered for easy
     * comparison.
     *
     * @generated from enum value: EDITION_2023 = 1000;
     */
    Edition[Edition["EDITION_2023"] = 1000] = "EDITION_2023";
    /**
     * @generated from enum value: EDITION_2024 = 1001;
     */
    Edition[Edition["EDITION_2024"] = 1001] = "EDITION_2024";
    /**
     * A placeholder edition for developing and testing unscheduled features.
     *
     * @generated from enum value: EDITION_UNSTABLE = 9999;
     */
    Edition[Edition["EDITION_UNSTABLE"] = 9999] = "EDITION_UNSTABLE";
    /**
     * Placeholder editions for testing feature resolution.  These should not be
     * used or relied on outside of tests.
     *
     * @generated from enum value: EDITION_1_TEST_ONLY = 1;
     */
    Edition[Edition["EDITION_1_TEST_ONLY"] = 1] = "EDITION_1_TEST_ONLY";
    /**
     * @generated from enum value: EDITION_2_TEST_ONLY = 2;
     */
    Edition[Edition["EDITION_2_TEST_ONLY"] = 2] = "EDITION_2_TEST_ONLY";
    /**
     * @generated from enum value: EDITION_99997_TEST_ONLY = 99997;
     */
    Edition[Edition["EDITION_99997_TEST_ONLY"] = 99997] = "EDITION_99997_TEST_ONLY";
    /**
     * @generated from enum value: EDITION_99998_TEST_ONLY = 99998;
     */
    Edition[Edition["EDITION_99998_TEST_ONLY"] = 99998] = "EDITION_99998_TEST_ONLY";
    /**
     * @generated from enum value: EDITION_99999_TEST_ONLY = 99999;
     */
    Edition[Edition["EDITION_99999_TEST_ONLY"] = 99999] = "EDITION_99999_TEST_ONLY";
    /**
     * Placeholder for specifying unbounded edition support.  This should only
     * ever be used by plugins that can expect to never require any changes to
     * support a new edition.
     *
     * @generated from enum value: EDITION_MAX = 2147483647;
     */
    Edition[Edition["EDITION_MAX"] = 2147483647] = "EDITION_MAX";
})(Edition || (Edition = {}));
/**
 * Describes the enum google.protobuf.Edition.
 */
const EditionSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (enumDesc(file_google_protobuf_descriptor, 0)));
/**
 * Describes the 'visibility' of a symbol with respect to the proto import
 * system. Symbols can only be imported when the visibility rules do not prevent
 * it (ex: local symbols cannot be imported).  Visibility modifiers can only set
 * on `message` and `enum` as they are the only types available to be referenced
 * from other files.
 *
 * @generated from enum google.protobuf.SymbolVisibility
 */
var SymbolVisibility;
(function (SymbolVisibility) {
    /**
     * @generated from enum value: VISIBILITY_UNSET = 0;
     */
    SymbolVisibility[SymbolVisibility["VISIBILITY_UNSET"] = 0] = "VISIBILITY_UNSET";
    /**
     * @generated from enum value: VISIBILITY_LOCAL = 1;
     */
    SymbolVisibility[SymbolVisibility["VISIBILITY_LOCAL"] = 1] = "VISIBILITY_LOCAL";
    /**
     * @generated from enum value: VISIBILITY_EXPORT = 2;
     */
    SymbolVisibility[SymbolVisibility["VISIBILITY_EXPORT"] = 2] = "VISIBILITY_EXPORT";
})(SymbolVisibility || (SymbolVisibility = {}));
/**
 * Describes the enum google.protobuf.SymbolVisibility.
 */
const SymbolVisibilitySchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (enumDesc(file_google_protobuf_descriptor, 1)));


/***/ }),

/***/ 9103:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  u: () => (/* binding */ runStreamingCall),
  L: () => (/* binding */ runUnaryCall)
});

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/interceptor.js
// Copyright 2021-2025 The Connect Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
/**
 * applyInterceptors takes the given UnaryFn or ServerStreamingFn, and wraps
 * it with each of the given interceptors, returning a new UnaryFn or
 * ServerStreamingFn.
 */
function applyInterceptors(next, interceptors) {
    if (!interceptors) {
        return next;
    }
    for (const i of interceptors.concat().reverse()) {
        next = i(next);
    }
    return next;
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/connect-error.js
var connect_error = __webpack_require__(45640);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/code.js
var code = __webpack_require__(16294);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/protocol/signals.js
// Copyright 2021-2025 The Connect Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.


/**
 * Create an AbortController that is automatically aborted if one of the given
 * signals is aborted.
 *
 * For convenience, the linked AbortSignals can be undefined.
 *
 * If the controller or any of the signals is aborted, all event listeners are
 * removed.
 *
 * @private Internal code, does not follow semantic versioning.
 */
function createLinkedAbortController(...signals) {
    const controller = new AbortController();
    const sa = signals.filter((s) => s !== undefined).concat(controller.signal);
    for (const signal of sa) {
        if (signal.aborted) {
            onAbort.apply(signal);
            break;
        }
        signal.addEventListener("abort", onAbort);
    }
    function onAbort() {
        if (!controller.signal.aborted) {
            controller.abort(getAbortSignalReason(this));
        }
        for (const signal of sa) {
            signal.removeEventListener("abort", onAbort);
        }
    }
    return controller;
}
/**
 * Create a deadline signal. The returned object contains an AbortSignal, but
 * also a cleanup function to stop the timer, which must be called once the
 * calling code is no longer interested in the signal.
 *
 * Ideally, we would simply use AbortSignal.timeout(), but it is not widely
 * available yet.
 *
 * @private Internal code, does not follow semantic versioning.
 */
function createDeadlineSignal(timeoutMs) {
    const controller = new AbortController();
    const listener = () => {
        controller.abort(new connect_error/* ConnectError */.T("the operation timed out", code/* Code */.C.DeadlineExceeded));
    };
    let timeoutId;
    if (timeoutMs !== undefined) {
        if (timeoutMs <= 0)
            listener();
        else
            timeoutId = setTimeout(listener, timeoutMs);
    }
    return {
        signal: controller.signal,
        cleanup: () => clearTimeout(timeoutId),
    };
}
/**
 * Returns the reason why an AbortSignal was aborted. Returns undefined if the
 * signal has not been aborted.
 *
 * The property AbortSignal.reason is not widely available. This function
 * returns an AbortError if the signal is aborted, but reason is undefined.
 *
 * @private Internal code, does not follow semantic versioning.
 */
function getAbortSignalReason(signal) {
    if (!signal.aborted) {
        return undefined;
    }
    if (signal.reason !== undefined) {
        return signal.reason;
    }
    // AbortSignal.reason is available in Node.js v16, v18, and later,
    // and in all browsers since early 2022.
    const e = new Error("This operation was aborted");
    e.name = "AbortError";
    return e;
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/create.js
var create = __webpack_require__(95444);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/protocol/normalize.js
// Copyright 2021-2025 The Connect Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

/**
 *  Takes a partial protobuf messages of the
 *  specified message type as input, and returns full instances.
 */
function normalize(desc, message) {
    return (0,create/* create */.v)(desc, message);
}
/**
 * Takes an AsyncIterable of partial protobuf messages of the
 * specified message type as input, and yields full instances.
 */
function normalizeIterable(desc, input) {
    function transform(result) {
        if (result.done === true) {
            return result;
        }
        return {
            done: result.done,
            value: normalize(desc, result.value),
        };
    }
    return {
        [Symbol.asyncIterator]() {
            const it = input[Symbol.asyncIterator]();
            const res = {
                next: () => it.next().then(transform),
            };
            if (it.throw !== undefined) {
                res.throw = (e) => it.throw(e).then(transform);
            }
            if (it.return !== undefined) {
                res.return = (v) => it.return(v).then(transform);
            }
            return res;
        },
    };
}

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/protocol/run-call.js
// Copyright 2021-2025 The Connect Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.




/**
 * Runs a unary method with the given interceptors. Note that this function
 * is only used when implementing a Transport.
 */
function runUnaryCall(opt) {
    const next = applyInterceptors(opt.next, opt.interceptors);
    const [signal, abort, done] = setupSignal(opt);
    const req = Object.assign(Object.assign({}, opt.req), { message: normalize(opt.req.method.input, opt.req.message), signal });
    return next(req).then((res) => {
        done();
        return res;
    }, abort);
}
/**
 * Runs a server-streaming method with the given interceptors. Note that this
 * function is only used when implementing a Transport.
 */
function runStreamingCall(opt) {
    const next = applyInterceptors(opt.next, opt.interceptors);
    const [signal, abort, done] = setupSignal(opt);
    const req = Object.assign(Object.assign({}, opt.req), { message: normalizeIterable(opt.req.method.input, opt.req.message), signal });
    let doneCalled = false;
    // Call return on the request iterable to indicate
    // that we will no longer consume it and it should
    // cleanup any allocated resources.
    signal.addEventListener("abort", function () {
        var _a, _b;
        const it = opt.req.message[Symbol.asyncIterator]();
        // If the signal is aborted due to an error, we want to throw
        // the error to the request iterator.
        if (!doneCalled) {
            (_a = it.throw) === null || _a === void 0 ? void 0 : _a.call(it, this.reason).catch(() => {
                // throw returns a promise, which we don't care about.
                //
                // Uncaught promises are thrown at sometime/somewhere by the event loop,
                // this is to ensure error is caught and ignored.
            });
        }
        (_b = it.return) === null || _b === void 0 ? void 0 : _b.call(it).catch(() => {
            // return returns a promise, which we don't care about.
            //
            // Uncaught promises are thrown at sometime/somewhere by the event loop,
            // this is to ensure error is caught and ignored.
        });
    });
    return next(req).then((res) => {
        return Object.assign(Object.assign({}, res), { message: {
                [Symbol.asyncIterator]() {
                    const it = res.message[Symbol.asyncIterator]();
                    return {
                        next() {
                            return it.next().then((r) => {
                                if (r.done == true) {
                                    doneCalled = true;
                                    done();
                                }
                                return r;
                            }, abort);
                        },
                        // We deliberately omit throw/return.
                    };
                },
            } });
    }, abort);
}
/**
 * Create an AbortSignal for Transport implementations. The signal is available
 * in UnaryRequest and StreamingRequest, and is triggered when the call is
 * aborted (via a timeout or explicit cancellation), errored (e.g. when reading
 * an error from the server from the wire), or finished successfully.
 *
 * Transport implementations can pass the signal to HTTP clients to ensure that
 * there are no unused connections leak.
 *
 * Returns a tuple:
 * [0]: The signal, which is also aborted if the optional deadline is reached.
 * [1]: Function to call if the Transport encountered an error.
 * [2]: Function to call if the Transport finished without an error.
 */
function setupSignal(opt) {
    const { signal, cleanup } = createDeadlineSignal(opt.timeoutMs);
    const controller = createLinkedAbortController(opt.signal, signal);
    return [
        controller.signal,
        function abort(reason) {
            // We peek at the deadline signal because fetch() will throw an error on
            // abort that discards the signal reason.
            const e = connect_error/* ConnectError */.T.from(signal.aborted ? getAbortSignalReason(signal) : reason);
            controller.abort(e);
            cleanup();
            return Promise.reject(e);
        },
        function done() {
            cleanup();
            controller.abort();
        },
    ];
}


/***/ }),

/***/ 10706:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  G0: () => (/* binding */ anyPack),
  rW: () => (/* binding */ anyUnpack)
});

// UNUSED EXPORTS: anyIs, anyUnpackTo

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/codegenv2/file.js
var file = __webpack_require__(1844);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/codegenv2/message.js
var message = __webpack_require__(15033);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/wkt/gen/google/protobuf/any_pb.js
// Copyright 2021-2026 Buf Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.


/**
 * Describes the file google/protobuf/any.proto.
 */
const file_google_protobuf_any = /*@__PURE__*/ (0,file/* fileDesc */.w)("Chlnb29nbGUvcHJvdG9idWYvYW55LnByb3RvEg9nb29nbGUucHJvdG9idWYiJgoDQW55EhAKCHR5cGVfdXJsGAEgASgJEg0KBXZhbHVlGAIgASgMQnYKE2NvbS5nb29nbGUucHJvdG9idWZCCEFueVByb3RvUAFaLGdvb2dsZS5nb2xhbmcub3JnL3Byb3RvYnVmL3R5cGVzL2tub3duL2FueXBiogIDR1BCqgIeR29vZ2xlLlByb3RvYnVmLldlbGxLbm93blR5cGVzYgZwcm90bzM");
/**
 * Describes the message google.protobuf.Any.
 * Use `create(AnySchema)` to create a new message.
 */
const AnySchema = /*@__PURE__*/ (0,message/* messageDesc */.Z)(file_google_protobuf_any, 0);

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/create.js
var create = __webpack_require__(95444);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/to-binary.js
var to_binary = __webpack_require__(2681);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/from-binary.js
var from_binary = __webpack_require__(57386);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/wkt/any.js
// Copyright 2021-2026 Buf Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.




function anyPack(schema, message, into) {
    let ret = false;
    if (!into) {
        into = (0,create/* create */.v)(AnySchema);
        ret = true;
    }
    into.value = (0,to_binary/* toBinary */.R)(schema, message);
    into.typeUrl = typeNameToUrl(message.$typeName);
    return ret ? into : undefined;
}
function anyIs(any, descOrTypeName) {
    if (any.typeUrl === "") {
        return false;
    }
    const want = typeof descOrTypeName == "string"
        ? descOrTypeName
        : descOrTypeName.typeName;
    const got = typeUrlToName(any.typeUrl);
    return want === got;
}
function anyUnpack(any, registryOrMessageDesc) {
    if (any.typeUrl === "") {
        return undefined;
    }
    const desc = registryOrMessageDesc.kind == "message"
        ? registryOrMessageDesc
        : registryOrMessageDesc.getMessage(typeUrlToName(any.typeUrl));
    if (!desc || !anyIs(any, desc)) {
        return undefined;
    }
    return (0,from_binary/* fromBinary */.Er)(desc, any.value);
}
/**
 * Same as anyUnpack but unpacks into the target message.
 */
function anyUnpackTo(any, schema, message) {
    if (!anyIs(any, schema)) {
        return undefined;
    }
    return mergeFromBinary(schema, message, any.value);
}
function typeNameToUrl(name) {
    return `type.googleapis.com/${name}`;
}
function typeUrlToName(url) {
    const slash = url.lastIndexOf("/");
    const name = slash >= 0 ? url.substring(slash + 1) : url;
    if (!name.length) {
        throw new Error(`invalid type url: ${url}`);
    }
    return name;
}


/***/ }),

/***/ 12140:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   h: () => (/* binding */ restoreJsonNames)
/* harmony export */ });
/* harmony import */ var _reflect_names_js__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(35250);
/* harmony import */ var _reflect_unsafe_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(22060);
// Copyright 2021-2026 Buf Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.


/**
 * @private
 */
function restoreJsonNames(message) {
    for (const f of message.field) {
        if (!(0,_reflect_unsafe_js__WEBPACK_IMPORTED_MODULE_0__/* .unsafeIsSetExplicit */ .b1)(f, "jsonName")) {
            f.jsonName = (0,_reflect_names_js__WEBPACK_IMPORTED_MODULE_1__/* .protoCamelCase */ .RM)(f.name);
        }
    }
    message.nestedType.forEach(restoreJsonNames);
}


/***/ }),

/***/ 13399:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   k: () => (/* binding */ createContextValues)
/* harmony export */ });
/* unused harmony export createContextKey */
// Copyright 2021-2025 The Connect Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
/**
 * createContextValues creates a new ContextValues.
 */
function createContextValues() {
    return {
        get(key) {
            return key.id in this ? this[key.id] : key.defaultValue;
        },
        set(key, value) {
            this[key.id] = value;
            return this;
        },
        delete(key) {
            delete this[key.id];
            return this;
        },
    };
}
/**
 * createContextKey creates a new ContextKey.
 */
function createContextKey(defaultValue, options) {
    return { id: Symbol(options === null || options === void 0 ? void 0 : options.description), defaultValue };
}


/***/ }),

/***/ 14097:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   K: () => (/* binding */ base64Encode),
/* harmony export */   u: () => (/* binding */ base64Decode)
/* harmony export */ });
// Copyright 2021-2026 Buf Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
/**
 * Decodes a base64 string to a byte array.
 *
 * - ignores white-space, including line breaks and tabs
 * - allows inner padding (can decode concatenated base64 strings)
 * - does not require padding
 * - understands base64url encoding:
 *   "-" instead of "+",
 *   "_" instead of "/",
 *   no padding
 */
function base64Decode(base64Str) {
    const table = getDecodeTable();
    // estimate byte size, not accounting for inner padding and whitespace
    let es = (base64Str.length * 3) / 4;
    if (base64Str[base64Str.length - 2] == "=")
        es -= 2;
    else if (base64Str[base64Str.length - 1] == "=")
        es -= 1;
    let bytes = new Uint8Array(es), bytePos = 0, // position in byte array
    groupPos = 0, // position in base64 group
    b, // current byte
    p = 0; // previous byte
    for (let i = 0; i < base64Str.length; i++) {
        b = table[base64Str.charCodeAt(i)];
        if (b === undefined) {
            switch (base64Str[i]) {
                // @ts-ignore TS7029: Fallthrough case in switch -- ignore instead of expect-error for compiler settings without noFallthroughCasesInSwitch: true
                case "=":
                    groupPos = 0; // reset state when padding found
                case "\n":
                case "\r":
                case "\t":
                case " ":
                    continue; // skip white-space, and padding
                default:
                    throw Error("invalid base64 string");
            }
        }
        switch (groupPos) {
            case 0:
                p = b;
                groupPos = 1;
                break;
            case 1:
                bytes[bytePos++] = (p << 2) | ((b & 48) >> 4);
                p = b;
                groupPos = 2;
                break;
            case 2:
                bytes[bytePos++] = ((p & 15) << 4) | ((b & 60) >> 2);
                p = b;
                groupPos = 3;
                break;
            case 3:
                bytes[bytePos++] = ((p & 3) << 6) | b;
                groupPos = 0;
                break;
        }
    }
    if (groupPos == 1)
        throw Error("invalid base64 string");
    return bytes.subarray(0, bytePos);
}
/**
 * Encode a byte array to a base64 string.
 *
 * By default, this function uses the standard base64 encoding with padding.
 *
 * To encode without padding, use encoding = "std_raw".
 *
 * To encode with the URL encoding, use encoding = "url", which replaces the
 * characters +/ by their URL-safe counterparts -_, and omits padding.
 */
function base64Encode(bytes, encoding = "std") {
    const table = getEncodeTable(encoding);
    const pad = encoding == "std";
    let base64 = "", groupPos = 0, // position in base64 group
    b, // current byte
    p = 0; // carry over from previous byte
    for (let i = 0; i < bytes.length; i++) {
        b = bytes[i];
        switch (groupPos) {
            case 0:
                base64 += table[b >> 2];
                p = (b & 3) << 4;
                groupPos = 1;
                break;
            case 1:
                base64 += table[p | (b >> 4)];
                p = (b & 15) << 2;
                groupPos = 2;
                break;
            case 2:
                base64 += table[p | (b >> 6)];
                base64 += table[b & 63];
                groupPos = 0;
                break;
        }
    }
    // add output padding
    if (groupPos) {
        base64 += table[p];
        if (pad) {
            base64 += "=";
            if (groupPos == 1)
                base64 += "=";
        }
    }
    return base64;
}
// lookup table from base64 character to byte
let encodeTableStd;
let encodeTableUrl;
// lookup table from base64 character *code* to byte because lookup by number is fast
let decodeTable;
function getEncodeTable(encoding) {
    if (!encodeTableStd) {
        encodeTableStd =
            "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/".split("");
        encodeTableUrl = encodeTableStd.slice(0, -2).concat("-", "_");
    }
    return encoding == "url"
        ? // biome-ignore lint/style/noNonNullAssertion: TS fails to narrow down
            encodeTableUrl
        : encodeTableStd;
}
function getDecodeTable() {
    if (!decodeTable) {
        decodeTable = [];
        const encodeTable = getEncodeTable("std");
        for (let i = 0; i < encodeTable.length; i++)
            decodeTable[encodeTable[i].charCodeAt(0)] = i;
        // support base64url variants
        decodeTable["-".charCodeAt(0)] = encodeTable.indexOf("+");
        decodeTable["_".charCodeAt(0)] = encodeTable.indexOf("/");
    }
    return decodeTable;
}


/***/ }),

/***/ 15033:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Z: () => (/* binding */ messageDesc)
/* harmony export */ });
// Copyright 2021-2026 Buf Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
/**
 * Hydrate a message descriptor.
 *
 * @private
 */
function messageDesc(file, path, ...paths) {
    return paths.reduce((acc, cur) => acc.nestedMessages[cur], file.messages[path]);
}


/***/ }),

/***/ 31503:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  uO: () => (/* binding */ createFileRegistry)
});

// UNUSED EXPORTS: createMutableRegistry, createRegistry, maximumEdition, minimumEdition

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/descriptors.js
var descriptors = __webpack_require__(92806);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/proto-int64.js
var proto_int64 = __webpack_require__(2952);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/wire/text-format.js
// Copyright 2021-2026 Buf Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.


/**
 * Parse an enum value from the Protobuf text format.
 *
 * @private
 */
function parseTextFormatEnumValue(descEnum, value) {
    const enumValue = descEnum.values.find((v) => v.name === value);
    if (!enumValue) {
        throw new Error(`cannot parse ${descEnum} default value: ${value}`);
    }
    return enumValue.number;
}
/**
 * Parse a scalar value from the Protobuf text format.
 *
 * @private
 */
function parseTextFormatScalarValue(type, value) {
    switch (type) {
        case descriptors/* ScalarType */.L.STRING:
            return value;
        case descriptors/* ScalarType */.L.BYTES: {
            const u = unescapeBytesDefaultValue(value);
            if (u === false) {
                throw new Error(`cannot parse ${descriptors/* ScalarType */.L[type]} default value: ${value}`);
            }
            return u;
        }
        case descriptors/* ScalarType */.L.INT64:
        case descriptors/* ScalarType */.L.SFIXED64:
        case descriptors/* ScalarType */.L.SINT64:
            return proto_int64/* protoInt64 */.M.parse(value);
        case descriptors/* ScalarType */.L.UINT64:
        case descriptors/* ScalarType */.L.FIXED64:
            return proto_int64/* protoInt64 */.M.uParse(value);
        case descriptors/* ScalarType */.L.DOUBLE:
        case descriptors/* ScalarType */.L.FLOAT:
            switch (value) {
                case "inf":
                    return Number.POSITIVE_INFINITY;
                case "-inf":
                    return Number.NEGATIVE_INFINITY;
                case "nan":
                    return Number.NaN;
                default:
                    return parseFloat(value);
            }
        case descriptors/* ScalarType */.L.BOOL:
            return value === "true";
        case descriptors/* ScalarType */.L.INT32:
        case descriptors/* ScalarType */.L.UINT32:
        case descriptors/* ScalarType */.L.SINT32:
        case descriptors/* ScalarType */.L.FIXED32:
        case descriptors/* ScalarType */.L.SFIXED32:
            return parseInt(value, 10);
    }
}
/**
 * Parses a text-encoded default value (proto2) of a BYTES field.
 */
function unescapeBytesDefaultValue(str) {
    const b = [];
    const input = {
        tail: str,
        c: "",
        next() {
            if (this.tail.length == 0) {
                return false;
            }
            this.c = this.tail[0];
            this.tail = this.tail.substring(1);
            return true;
        },
        take(n) {
            if (this.tail.length >= n) {
                const r = this.tail.substring(0, n);
                this.tail = this.tail.substring(n);
                return r;
            }
            return false;
        },
    };
    while (input.next()) {
        switch (input.c) {
            case "\\":
                if (input.next()) {
                    switch (input.c) {
                        case "\\":
                            b.push(input.c.charCodeAt(0));
                            break;
                        case "b":
                            b.push(0x08);
                            break;
                        case "f":
                            b.push(0x0c);
                            break;
                        case "n":
                            b.push(0x0a);
                            break;
                        case "r":
                            b.push(0x0d);
                            break;
                        case "t":
                            b.push(0x09);
                            break;
                        case "v":
                            b.push(0x0b);
                            break;
                        case "0":
                        case "1":
                        case "2":
                        case "3":
                        case "4":
                        case "5":
                        case "6":
                        case "7": {
                            const s = input.c;
                            const t = input.take(2);
                            if (t === false) {
                                return false;
                            }
                            const n = parseInt(s + t, 8);
                            if (Number.isNaN(n)) {
                                return false;
                            }
                            b.push(n);
                            break;
                        }
                        case "x": {
                            const s = input.c;
                            const t = input.take(2);
                            if (t === false) {
                                return false;
                            }
                            const n = parseInt(s + t, 16);
                            if (Number.isNaN(n)) {
                                return false;
                            }
                            b.push(n);
                            break;
                        }
                        case "u": {
                            const s = input.c;
                            const t = input.take(4);
                            if (t === false) {
                                return false;
                            }
                            const n = parseInt(s + t, 16);
                            if (Number.isNaN(n)) {
                                return false;
                            }
                            const chunk = new Uint8Array(4);
                            const view = new DataView(chunk.buffer);
                            view.setInt32(0, n, true);
                            b.push(chunk[0], chunk[1], chunk[2], chunk[3]);
                            break;
                        }
                        case "U": {
                            const s = input.c;
                            const t = input.take(8);
                            if (t === false) {
                                return false;
                            }
                            const tc = proto_int64/* protoInt64 */.M.uEnc(s + t);
                            const chunk = new Uint8Array(8);
                            const view = new DataView(chunk.buffer);
                            view.setInt32(0, tc.lo, true);
                            view.setInt32(4, tc.hi, true);
                            b.push(chunk[0], chunk[1], chunk[2], chunk[3], chunk[4], chunk[5], chunk[6], chunk[7]);
                            break;
                        }
                    }
                }
                break;
            default:
                b.push(input.c.charCodeAt(0));
        }
    }
    return new Uint8Array(b);
}

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/reflect/nested-types.js
// Copyright 2021-2026 Buf Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
/**
 * Iterate over all types - enumerations, extensions, services, messages -
 * and enumerations, extensions and messages nested in messages.
 */
function* nestedTypes(desc) {
    switch (desc.kind) {
        case "file":
            for (const message of desc.messages) {
                yield message;
                yield* nestedTypes(message);
            }
            yield* desc.enums;
            yield* desc.services;
            yield* desc.extensions;
            break;
        case "message":
            for (const message of desc.nestedMessages) {
                yield message;
                yield* nestedTypes(message);
            }
            yield* desc.nestedEnums;
            yield* desc.nestedExtensions;
            break;
    }
}
/**
 * Iterate over types referenced by fields of the given message.
 *
 * For example:
 *
 * ```proto
 * syntax="proto3";
 *
 * message Example {
 *   Msg singular = 1;
 *   repeated Level list = 2;
 * }
 *
 * message Msg {}
 *
 * enum Level {
 *   LEVEL_UNSPECIFIED = 0;
 * }
 * ```
 *
 * The message Example references the message Msg, and the enum Level.
 */
function usedTypes(descMessage) {
    return usedTypesInternal(descMessage, new Set());
}
function* usedTypesInternal(descMessage, seen) {
    var _a, _b;
    for (const field of descMessage.fields) {
        const ref = (_b = (_a = field.enum) !== null && _a !== void 0 ? _a : field.message) !== null && _b !== void 0 ? _b : undefined;
        if (!ref || seen.has(ref.typeName)) {
            continue;
        }
        seen.add(ref.typeName);
        yield ref;
        if (ref.kind == "message") {
            yield* usedTypesInternal(ref, seen);
        }
    }
}
/**
 * Returns the ancestors of a given Protobuf element, up to the file.
 */
function parentTypes(desc) {
    const parents = [];
    while (desc.kind !== "file") {
        const p = nested_types_parent(desc);
        desc = p;
        parents.push(p);
    }
    return parents;
}
function nested_types_parent(desc) {
    var _a;
    switch (desc.kind) {
        case "enum_value":
        case "field":
        case "oneof":
        case "rpc":
            return desc.parent;
        case "service":
            return desc.file;
        case "extension":
        case "enum":
        case "message":
            return (_a = desc.parent) !== null && _a !== void 0 ? _a : desc.file;
    }
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/reflect/unsafe.js
var unsafe = __webpack_require__(22060);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/reflect/names.js
var names = __webpack_require__(35250);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/registry.js
// Copyright 2021-2026 Buf Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.





/**
 * Create a registry from the given inputs.
 *
 * An input can be:
 * - Any message, enum, service, or extension descriptor, which adds just the
 *   descriptor for this type.
 * - A file descriptor, which adds all typed defined in this file.
 * - A registry, which adds all types from the registry.
 *
 * For duplicate descriptors (same type name), the one given last wins.
 */
function createRegistry(...input) {
    return initBaseRegistry(input);
}
/**
 * Create a registry that allows adding and removing descriptors.
 */
function createMutableRegistry(...input) {
    const reg = initBaseRegistry(input);
    return Object.assign(Object.assign({}, reg), { remove(desc) {
            var _a;
            if (desc.kind == "extension") {
                (_a = reg.extendees.get(desc.extendee.typeName)) === null || _a === void 0 ? void 0 : _a.delete(desc.number);
            }
            reg.types.delete(desc.typeName);
        } });
}
function createFileRegistry(...args) {
    const registry = createBaseRegistry();
    if (!args.length) {
        return registry;
    }
    if ("$typeName" in args[0] &&
        args[0].$typeName == "google.protobuf.FileDescriptorSet") {
        for (const file of args[0].file) {
            addFile(file, registry);
        }
        return registry;
    }
    if ("$typeName" in args[0]) {
        const input = args[0];
        const resolve = args[1];
        const seen = new Set();
        function recurseDeps(file) {
            const deps = [];
            for (const protoFileName of file.dependency) {
                if (registry.getFile(protoFileName) != undefined) {
                    continue;
                }
                if (seen.has(protoFileName)) {
                    continue;
                }
                const dep = resolve(protoFileName);
                if (!dep) {
                    throw new Error(`Unable to resolve ${protoFileName}, imported by ${file.name}`);
                }
                if ("kind" in dep) {
                    registry.addFile(dep, false, true);
                }
                else {
                    seen.add(dep.name);
                    deps.push(dep);
                }
            }
            return deps.concat(...deps.map(recurseDeps));
        }
        for (const file of [input, ...recurseDeps(input)].reverse()) {
            addFile(file, registry);
        }
    }
    else {
        for (const fileReg of args) {
            for (const file of fileReg.files) {
                registry.addFile(file);
            }
        }
    }
    return registry;
}
/**
 * @private
 */
function createBaseRegistry() {
    const types = new Map();
    const extendees = new Map();
    const files = new Map();
    return {
        kind: "registry",
        types,
        extendees,
        [Symbol.iterator]() {
            return types.values();
        },
        get files() {
            return files.values();
        },
        addFile(file, skipTypes, withDeps) {
            files.set(file.proto.name, file);
            if (!skipTypes) {
                for (const type of nestedTypes(file)) {
                    this.add(type);
                }
            }
            if (withDeps) {
                for (const f of file.dependencies) {
                    this.addFile(f, skipTypes, withDeps);
                }
            }
        },
        add(desc) {
            if (desc.kind == "extension") {
                let numberToExt = extendees.get(desc.extendee.typeName);
                if (!numberToExt) {
                    extendees.set(desc.extendee.typeName, 
                    // biome-ignore lint/suspicious/noAssignInExpressions: no
                    (numberToExt = new Map()));
                }
                numberToExt.set(desc.number, desc);
            }
            types.set(desc.typeName, desc);
        },
        get(typeName) {
            return types.get(typeName);
        },
        getFile(fileName) {
            return files.get(fileName);
        },
        getMessage(typeName) {
            const t = types.get(typeName);
            return (t === null || t === void 0 ? void 0 : t.kind) == "message" ? t : undefined;
        },
        getEnum(typeName) {
            const t = types.get(typeName);
            return (t === null || t === void 0 ? void 0 : t.kind) == "enum" ? t : undefined;
        },
        getExtension(typeName) {
            const t = types.get(typeName);
            return (t === null || t === void 0 ? void 0 : t.kind) == "extension" ? t : undefined;
        },
        getExtensionFor(extendee, no) {
            var _a;
            return (_a = extendees.get(extendee.typeName)) === null || _a === void 0 ? void 0 : _a.get(no);
        },
        getService(typeName) {
            const t = types.get(typeName);
            return (t === null || t === void 0 ? void 0 : t.kind) == "service" ? t : undefined;
        },
    };
}
/**
 * @private
 */
function initBaseRegistry(inputs) {
    const registry = createBaseRegistry();
    for (const input of inputs) {
        switch (input.kind) {
            case "registry":
                for (const n of input) {
                    registry.add(n);
                }
                break;
            case "file":
                registry.addFile(input);
                break;
            default:
                registry.add(input);
                break;
        }
    }
    return registry;
}
// bootstrap-inject google.protobuf.Edition.EDITION_PROTO2: const $name: Edition.$localName = $number;
const EDITION_PROTO2 = 998;
// bootstrap-inject google.protobuf.Edition.EDITION_PROTO3: const $name: Edition.$localName = $number;
const EDITION_PROTO3 = 999;
// bootstrap-inject google.protobuf.FieldDescriptorProto.Type.TYPE_STRING: const $name: FieldDescriptorProto_Type.$localName = $number;
const TYPE_STRING = 9;
// bootstrap-inject google.protobuf.FieldDescriptorProto.Type.TYPE_GROUP: const $name: FieldDescriptorProto_Type.$localName = $number;
const TYPE_GROUP = 10;
// bootstrap-inject google.protobuf.FieldDescriptorProto.Type.TYPE_MESSAGE: const $name: FieldDescriptorProto_Type.$localName = $number;
const TYPE_MESSAGE = 11;
// bootstrap-inject google.protobuf.FieldDescriptorProto.Type.TYPE_BYTES: const $name: FieldDescriptorProto_Type.$localName = $number;
const TYPE_BYTES = 12;
// bootstrap-inject google.protobuf.FieldDescriptorProto.Type.TYPE_ENUM: const $name: FieldDescriptorProto_Type.$localName = $number;
const TYPE_ENUM = 14;
// bootstrap-inject google.protobuf.FieldDescriptorProto.Label.LABEL_REPEATED: const $name: FieldDescriptorProto_Label.$localName = $number;
const LABEL_REPEATED = 3;
// bootstrap-inject google.protobuf.FieldDescriptorProto.Label.LABEL_REQUIRED: const $name: FieldDescriptorProto_Label.$localName = $number;
const LABEL_REQUIRED = 2;
// bootstrap-inject google.protobuf.FieldOptions.JSType.JS_STRING: const $name: FieldOptions_JSType.$localName = $number;
const JS_STRING = 1;
// bootstrap-inject google.protobuf.MethodOptions.IdempotencyLevel.IDEMPOTENCY_UNKNOWN: const $name: MethodOptions_IdempotencyLevel.$localName = $number;
const IDEMPOTENCY_UNKNOWN = 0;
// bootstrap-inject google.protobuf.FeatureSet.FieldPresence.EXPLICIT: const $name: FeatureSet_FieldPresence.$localName = $number;
const EXPLICIT = 1;
// bootstrap-inject google.protobuf.FeatureSet.FieldPresence.IMPLICIT: const $name: FeatureSet_FieldPresence.$localName = $number;
const IMPLICIT = 2;
// bootstrap-inject google.protobuf.FeatureSet.FieldPresence.LEGACY_REQUIRED: const $name: FeatureSet_FieldPresence.$localName = $number;
const LEGACY_REQUIRED = 3;
// bootstrap-inject google.protobuf.FeatureSet.RepeatedFieldEncoding.PACKED: const $name: FeatureSet_RepeatedFieldEncoding.$localName = $number;
const PACKED = 1;
// bootstrap-inject google.protobuf.FeatureSet.MessageEncoding.DELIMITED: const $name: FeatureSet_MessageEncoding.$localName = $number;
const DELIMITED = 2;
// bootstrap-inject google.protobuf.FeatureSet.EnumType.OPEN: const $name: FeatureSet_EnumType.$localName = $number;
const OPEN = 1;
// biome-ignore format: want this to read well
// bootstrap-inject defaults: EDITION_PROTO2 to EDITION_2024: export const minimumEdition: SupportedEdition = $minimumEdition, maximumEdition: SupportedEdition = $maximumEdition;
// generated from protoc v33.2
const minimumEdition = 998, maximumEdition = 1001;
const featureDefaults = {
    // EDITION_PROTO2
    998: {
        fieldPresence: 1, // EXPLICIT,
        enumType: 2, // CLOSED,
        repeatedFieldEncoding: 2, // EXPANDED,
        utf8Validation: 3, // NONE,
        messageEncoding: 1, // LENGTH_PREFIXED,
        jsonFormat: 2, // LEGACY_BEST_EFFORT,
        enforceNamingStyle: 2, // STYLE_LEGACY,
        defaultSymbolVisibility: 1, // EXPORT_ALL,
    },
    // EDITION_PROTO3
    999: {
        fieldPresence: 2, // IMPLICIT,
        enumType: 1, // OPEN,
        repeatedFieldEncoding: 1, // PACKED,
        utf8Validation: 2, // VERIFY,
        messageEncoding: 1, // LENGTH_PREFIXED,
        jsonFormat: 1, // ALLOW,
        enforceNamingStyle: 2, // STYLE_LEGACY,
        defaultSymbolVisibility: 1, // EXPORT_ALL,
    },
    // EDITION_2023
    1000: {
        fieldPresence: 1, // EXPLICIT,
        enumType: 1, // OPEN,
        repeatedFieldEncoding: 1, // PACKED,
        utf8Validation: 2, // VERIFY,
        messageEncoding: 1, // LENGTH_PREFIXED,
        jsonFormat: 1, // ALLOW,
        enforceNamingStyle: 2, // STYLE_LEGACY,
        defaultSymbolVisibility: 1, // EXPORT_ALL,
    },
    // EDITION_2024
    1001: {
        fieldPresence: 1, // EXPLICIT,
        enumType: 1, // OPEN,
        repeatedFieldEncoding: 1, // PACKED,
        utf8Validation: 2, // VERIFY,
        messageEncoding: 1, // LENGTH_PREFIXED,
        jsonFormat: 1, // ALLOW,
        enforceNamingStyle: 1, // STYLE2024,
        defaultSymbolVisibility: 2, // EXPORT_TOP_LEVEL,
    },
};
/**
 * Create a descriptor for a file, add it to the registry.
 */
function addFile(proto, reg) {
    var _a, _b;
    const file = {
        kind: "file",
        proto,
        deprecated: (_b = (_a = proto.options) === null || _a === void 0 ? void 0 : _a.deprecated) !== null && _b !== void 0 ? _b : false,
        edition: getFileEdition(proto),
        name: proto.name.replace(/\.proto$/, ""),
        dependencies: findFileDependencies(proto, reg),
        enums: [],
        messages: [],
        extensions: [],
        services: [],
        toString() {
            // eslint-disable-next-line @typescript-eslint/restrict-template-expressions -- we asserted above
            return `file ${proto.name}`;
        },
    };
    const mapEntriesStore = new Map();
    const mapEntries = {
        get(typeName) {
            return mapEntriesStore.get(typeName);
        },
        add(desc) {
            var _a;
            assert(((_a = desc.proto.options) === null || _a === void 0 ? void 0 : _a.mapEntry) === true);
            mapEntriesStore.set(desc.typeName, desc);
        },
    };
    for (const enumProto of proto.enumType) {
        addEnum(enumProto, file, undefined, reg);
    }
    for (const messageProto of proto.messageType) {
        addMessage(messageProto, file, undefined, reg, mapEntries);
    }
    for (const serviceProto of proto.service) {
        addService(serviceProto, file, reg);
    }
    addExtensions(file, reg);
    for (const mapEntry of mapEntriesStore.values()) {
        // to create a map field, we need access to the map entry's fields
        addFields(mapEntry, reg, mapEntries);
    }
    for (const message of file.messages) {
        addFields(message, reg, mapEntries);
        addExtensions(message, reg);
    }
    reg.addFile(file, true);
}
/**
 * Create descriptors for extensions, and add them to the message / file,
 * and to our cart.
 * Recurses into nested types.
 */
function addExtensions(desc, reg) {
    switch (desc.kind) {
        case "file":
            for (const proto of desc.proto.extension) {
                const ext = newField(proto, desc, reg);
                desc.extensions.push(ext);
                reg.add(ext);
            }
            break;
        case "message":
            for (const proto of desc.proto.extension) {
                const ext = newField(proto, desc, reg);
                desc.nestedExtensions.push(ext);
                reg.add(ext);
            }
            for (const message of desc.nestedMessages) {
                addExtensions(message, reg);
            }
            break;
    }
}
/**
 * Create descriptors for fields and oneof groups, and add them to the message.
 * Recurses into nested types.
 */
function addFields(message, reg, mapEntries) {
    const allOneofs = message.proto.oneofDecl.map((proto) => newOneof(proto, message));
    const oneofsSeen = new Set();
    for (const proto of message.proto.field) {
        const oneof = findOneof(proto, allOneofs);
        const field = newField(proto, message, reg, oneof, mapEntries);
        message.fields.push(field);
        message.field[field.localName] = field;
        if (oneof === undefined) {
            message.members.push(field);
        }
        else {
            oneof.fields.push(field);
            if (!oneofsSeen.has(oneof)) {
                oneofsSeen.add(oneof);
                message.members.push(oneof);
            }
        }
    }
    for (const oneof of allOneofs.filter((o) => oneofsSeen.has(o))) {
        message.oneofs.push(oneof);
    }
    for (const child of message.nestedMessages) {
        addFields(child, reg, mapEntries);
    }
}
/**
 * Create a descriptor for an enumeration, and add it our cart and to the
 * parent type, if any.
 */
function addEnum(proto, file, parent, reg) {
    var _a, _b, _c, _d, _e;
    const sharedPrefix = findEnumSharedPrefix(proto.name, proto.value);
    const desc = {
        kind: "enum",
        proto,
        deprecated: (_b = (_a = proto.options) === null || _a === void 0 ? void 0 : _a.deprecated) !== null && _b !== void 0 ? _b : false,
        file,
        parent,
        open: true,
        name: proto.name,
        typeName: makeTypeName(proto, parent, file),
        value: {},
        values: [],
        sharedPrefix,
        toString() {
            return `enum ${this.typeName}`;
        },
    };
    desc.open = isEnumOpen(desc);
    reg.add(desc);
    for (const p of proto.value) {
        const name = p.name;
        desc.values.push(
        // biome-ignore lint/suspicious/noAssignInExpressions: no
        (desc.value[p.number] = {
            kind: "enum_value",
            proto: p,
            deprecated: (_d = (_c = p.options) === null || _c === void 0 ? void 0 : _c.deprecated) !== null && _d !== void 0 ? _d : false,
            parent: desc,
            name,
            localName: (0,names/* safeObjectProperty */.aV)(sharedPrefix == undefined
                ? name
                : name.substring(sharedPrefix.length)),
            number: p.number,
            toString() {
                return `enum value ${desc.typeName}.${name}`;
            },
        }));
    }
    ((_e = parent === null || parent === void 0 ? void 0 : parent.nestedEnums) !== null && _e !== void 0 ? _e : file.enums).push(desc);
}
/**
 * Create a descriptor for a message, including nested types, and add it to our
 * cart. Note that this does not create descriptors fields.
 */
function addMessage(proto, file, parent, reg, mapEntries) {
    var _a, _b, _c, _d;
    const desc = {
        kind: "message",
        proto,
        deprecated: (_b = (_a = proto.options) === null || _a === void 0 ? void 0 : _a.deprecated) !== null && _b !== void 0 ? _b : false,
        file,
        parent,
        name: proto.name,
        typeName: makeTypeName(proto, parent, file),
        fields: [],
        field: {},
        oneofs: [],
        members: [],
        nestedEnums: [],
        nestedMessages: [],
        nestedExtensions: [],
        toString() {
            return `message ${this.typeName}`;
        },
    };
    if (((_c = proto.options) === null || _c === void 0 ? void 0 : _c.mapEntry) === true) {
        mapEntries.add(desc);
    }
    else {
        ((_d = parent === null || parent === void 0 ? void 0 : parent.nestedMessages) !== null && _d !== void 0 ? _d : file.messages).push(desc);
        reg.add(desc);
    }
    for (const enumProto of proto.enumType) {
        addEnum(enumProto, file, desc, reg);
    }
    for (const messageProto of proto.nestedType) {
        addMessage(messageProto, file, desc, reg, mapEntries);
    }
}
/**
 * Create a descriptor for a service, including methods, and add it to our
 * cart.
 */
function addService(proto, file, reg) {
    var _a, _b;
    const desc = {
        kind: "service",
        proto,
        deprecated: (_b = (_a = proto.options) === null || _a === void 0 ? void 0 : _a.deprecated) !== null && _b !== void 0 ? _b : false,
        file,
        name: proto.name,
        typeName: makeTypeName(proto, undefined, file),
        methods: [],
        method: {},
        toString() {
            return `service ${this.typeName}`;
        },
    };
    file.services.push(desc);
    reg.add(desc);
    for (const methodProto of proto.method) {
        const method = newMethod(methodProto, desc, reg);
        desc.methods.push(method);
        desc.method[method.localName] = method;
    }
}
/**
 * Create a descriptor for a method.
 */
function newMethod(proto, parent, reg) {
    var _a, _b, _c, _d;
    let methodKind;
    if (proto.clientStreaming && proto.serverStreaming) {
        methodKind = "bidi_streaming";
    }
    else if (proto.clientStreaming) {
        methodKind = "client_streaming";
    }
    else if (proto.serverStreaming) {
        methodKind = "server_streaming";
    }
    else {
        methodKind = "unary";
    }
    const input = reg.getMessage(trimLeadingDot(proto.inputType));
    const output = reg.getMessage(trimLeadingDot(proto.outputType));
    assert(input, `invalid MethodDescriptorProto: input_type ${proto.inputType} not found`);
    assert(output, `invalid MethodDescriptorProto: output_type ${proto.inputType} not found`);
    const name = proto.name;
    return {
        kind: "rpc",
        proto,
        deprecated: (_b = (_a = proto.options) === null || _a === void 0 ? void 0 : _a.deprecated) !== null && _b !== void 0 ? _b : false,
        parent,
        name,
        localName: (0,names/* safeObjectProperty */.aV)(name.length
            ? (0,names/* safeObjectProperty */.aV)(name[0].toLowerCase() + name.substring(1))
            : name),
        methodKind,
        input,
        output,
        idempotency: (_d = (_c = proto.options) === null || _c === void 0 ? void 0 : _c.idempotencyLevel) !== null && _d !== void 0 ? _d : IDEMPOTENCY_UNKNOWN,
        toString() {
            return `rpc ${parent.typeName}.${name}`;
        },
    };
}
/**
 * Create a descriptor for a oneof group.
 */
function newOneof(proto, parent) {
    return {
        kind: "oneof",
        proto,
        deprecated: false,
        parent,
        fields: [],
        name: proto.name,
        localName: (0,names/* safeObjectProperty */.aV)((0,names/* protoCamelCase */.RM)(proto.name)),
        toString() {
            return `oneof ${parent.typeName}.${this.name}`;
        },
    };
}
function newField(proto, parentOrFile, reg, oneof, mapEntries) {
    var _a, _b, _c;
    const isExtension = mapEntries === undefined;
    const field = {
        kind: "field",
        proto,
        deprecated: (_b = (_a = proto.options) === null || _a === void 0 ? void 0 : _a.deprecated) !== null && _b !== void 0 ? _b : false,
        name: proto.name,
        number: proto.number,
        scalar: undefined,
        message: undefined,
        enum: undefined,
        presence: getFieldPresence(proto, oneof, isExtension, parentOrFile),
        listKind: undefined,
        mapKind: undefined,
        mapKey: undefined,
        delimitedEncoding: undefined,
        packed: undefined,
        longAsString: false,
        getDefaultValue: undefined,
    };
    if (isExtension) {
        // extension field
        const file = parentOrFile.kind == "file" ? parentOrFile : parentOrFile.file;
        const parent = parentOrFile.kind == "file" ? undefined : parentOrFile;
        const typeName = makeTypeName(proto, parent, file);
        field.kind = "extension";
        field.file = file;
        field.parent = parent;
        field.oneof = undefined;
        field.typeName = typeName;
        field.jsonName = `[${typeName}]`; // option json_name is not allowed on extension fields
        field.toString = () => `extension ${typeName}`;
        const extendee = reg.getMessage(trimLeadingDot(proto.extendee));
        assert(extendee, `invalid FieldDescriptorProto: extendee ${proto.extendee} not found`);
        field.extendee = extendee;
    }
    else {
        // regular field
        const parent = parentOrFile;
        assert(parent.kind == "message");
        field.parent = parent;
        field.oneof = oneof;
        field.localName = oneof
            ? (0,names/* protoCamelCase */.RM)(proto.name)
            : (0,names/* safeObjectProperty */.aV)((0,names/* protoCamelCase */.RM)(proto.name));
        field.jsonName = proto.jsonName;
        field.toString = () => `field ${parent.typeName}.${proto.name}`;
    }
    const label = proto.label;
    const type = proto.type;
    const jstype = (_c = proto.options) === null || _c === void 0 ? void 0 : _c.jstype;
    if (label === LABEL_REPEATED) {
        // list or map field
        const mapEntry = type == TYPE_MESSAGE
            ? mapEntries === null || mapEntries === void 0 ? void 0 : mapEntries.get(trimLeadingDot(proto.typeName))
            : undefined;
        if (mapEntry) {
            // map field
            field.fieldKind = "map";
            const { key, value } = findMapEntryFields(mapEntry);
            field.mapKey = key.scalar;
            field.mapKind = value.fieldKind;
            field.message = value.message;
            field.delimitedEncoding = false; // map fields are always LENGTH_PREFIXED
            field.enum = value.enum;
            field.scalar = value.scalar;
            return field;
        }
        // list field
        field.fieldKind = "list";
        switch (type) {
            case TYPE_MESSAGE:
            case TYPE_GROUP:
                field.listKind = "message";
                field.message = reg.getMessage(trimLeadingDot(proto.typeName));
                assert(field.message);
                field.delimitedEncoding = isDelimitedEncoding(proto, parentOrFile);
                break;
            case TYPE_ENUM:
                field.listKind = "enum";
                field.enum = reg.getEnum(trimLeadingDot(proto.typeName));
                assert(field.enum);
                break;
            default:
                field.listKind = "scalar";
                field.scalar = type;
                field.longAsString = jstype == JS_STRING;
                break;
        }
        field.packed = isPackedField(proto, parentOrFile);
        return field;
    }
    // singular
    switch (type) {
        case TYPE_MESSAGE:
        case TYPE_GROUP:
            field.fieldKind = "message";
            field.message = reg.getMessage(trimLeadingDot(proto.typeName));
            assert(field.message, `invalid FieldDescriptorProto: type_name ${proto.typeName} not found`);
            field.delimitedEncoding = isDelimitedEncoding(proto, parentOrFile);
            field.getDefaultValue = () => undefined;
            break;
        case TYPE_ENUM: {
            const enumeration = reg.getEnum(trimLeadingDot(proto.typeName));
            assert(enumeration !== undefined, `invalid FieldDescriptorProto: type_name ${proto.typeName} not found`);
            field.fieldKind = "enum";
            field.enum = reg.getEnum(trimLeadingDot(proto.typeName));
            field.getDefaultValue = () => {
                return (0,unsafe/* unsafeIsSetExplicit */.b1)(proto, "defaultValue")
                    ? parseTextFormatEnumValue(enumeration, proto.defaultValue)
                    : undefined;
            };
            break;
        }
        default: {
            field.fieldKind = "scalar";
            field.scalar = type;
            field.longAsString = jstype == JS_STRING;
            field.getDefaultValue = () => {
                return (0,unsafe/* unsafeIsSetExplicit */.b1)(proto, "defaultValue")
                    ? parseTextFormatScalarValue(type, proto.defaultValue)
                    : undefined;
            };
            break;
        }
    }
    return field;
}
/**
 * Parse the "syntax" and "edition" fields, returning one of the supported
 * editions.
 */
function getFileEdition(proto) {
    switch (proto.syntax) {
        case "":
        case "proto2":
            return EDITION_PROTO2;
        case "proto3":
            return EDITION_PROTO3;
        case "editions":
            if (proto.edition in featureDefaults) {
                return proto.edition;
            }
            throw new Error(`${proto.name}: unsupported edition`);
        default:
            throw new Error(`${proto.name}: unsupported syntax "${proto.syntax}"`);
    }
}
/**
 * Resolve dependencies of FileDescriptorProto to DescFile.
 */
function findFileDependencies(proto, reg) {
    return proto.dependency.map((wantName) => {
        const dep = reg.getFile(wantName);
        if (!dep) {
            throw new Error(`Cannot find ${wantName}, imported by ${proto.name}`);
        }
        return dep;
    });
}
/**
 * Finds a prefix shared by enum values, for example `my_enum_` for
 * `enum MyEnum {MY_ENUM_A=0; MY_ENUM_B=1;}`.
 */
function findEnumSharedPrefix(enumName, values) {
    const prefix = camelToSnakeCase(enumName) + "_";
    for (const value of values) {
        if (!value.name.toLowerCase().startsWith(prefix)) {
            return undefined;
        }
        const shortName = value.name.substring(prefix.length);
        if (shortName.length == 0) {
            return undefined;
        }
        if (/^\d/.test(shortName)) {
            // identifiers must not start with numbers
            return undefined;
        }
    }
    return prefix;
}
/**
 * Converts lowerCamelCase or UpperCamelCase into lower_snake_case.
 * This is used to find shared prefixes in an enum.
 */
function camelToSnakeCase(camel) {
    return (camel.substring(0, 1) + camel.substring(1).replace(/[A-Z]/g, (c) => "_" + c)).toLowerCase();
}
/**
 * Create a fully qualified name for a protobuf type or extension field.
 *
 * The fully qualified name for messages, enumerations, and services is
 * constructed by concatenating the package name (if present), parent
 * message names (for nested types), and the type name. We omit the leading
 * dot added by protobuf compilers. Examples:
 * - mypackage.MyMessage
 * - mypackage.MyMessage.NestedMessage
 *
 * The fully qualified name for extension fields is constructed by
 * concatenating the package name (if present), parent message names (for
 * extensions declared within a message), and the field name. Examples:
 * - mypackage.extfield
 * - mypackage.MyMessage.extfield
 */
function makeTypeName(proto, parent, file) {
    let typeName;
    if (parent) {
        typeName = `${parent.typeName}.${proto.name}`;
    }
    else if (file.proto.package.length > 0) {
        typeName = `${file.proto.package}.${proto.name}`;
    }
    else {
        typeName = `${proto.name}`;
    }
    return typeName;
}
/**
 * Remove the leading dot from a fully qualified type name.
 */
function trimLeadingDot(typeName) {
    return typeName.startsWith(".") ? typeName.substring(1) : typeName;
}
/**
 * Did the user put the field in a oneof group?
 * Synthetic oneofs for proto3 optionals are ignored.
 */
function findOneof(proto, allOneofs) {
    if (!(0,unsafe/* unsafeIsSetExplicit */.b1)(proto, "oneofIndex")) {
        return undefined;
    }
    if (proto.proto3Optional) {
        return undefined;
    }
    const oneof = allOneofs[proto.oneofIndex];
    assert(oneof, `invalid FieldDescriptorProto: oneof #${proto.oneofIndex} for field #${proto.number} not found`);
    return oneof;
}
/**
 * Presence of the field.
 * See https://protobuf.dev/programming-guides/field_presence/
 */
function getFieldPresence(proto, oneof, isExtension, parent) {
    if (proto.label == LABEL_REQUIRED) {
        // proto2 required is LEGACY_REQUIRED
        return LEGACY_REQUIRED;
    }
    if (proto.label == LABEL_REPEATED) {
        // repeated fields (including maps) do not track presence
        return IMPLICIT;
    }
    if (!!oneof || proto.proto3Optional) {
        // oneof is always explicit
        return EXPLICIT;
    }
    if (isExtension) {
        // extensions always track presence
        return EXPLICIT;
    }
    const resolved = resolveFeature("fieldPresence", { proto, parent });
    if (resolved == IMPLICIT &&
        (proto.type == TYPE_MESSAGE || proto.type == TYPE_GROUP)) {
        // singular message field cannot be implicit
        return EXPLICIT;
    }
    return resolved;
}
/**
 * Pack this repeated field?
 */
function isPackedField(proto, parent) {
    if (proto.label != LABEL_REPEATED) {
        return false;
    }
    switch (proto.type) {
        case TYPE_STRING:
        case TYPE_BYTES:
        case TYPE_GROUP:
        case TYPE_MESSAGE:
            // length-delimited types cannot be packed
            return false;
    }
    const o = proto.options;
    if (o && (0,unsafe/* unsafeIsSetExplicit */.b1)(o, "packed")) {
        // prefer the field option over edition features
        return o.packed;
    }
    return (PACKED ==
        resolveFeature("repeatedFieldEncoding", {
            proto,
            parent,
        }));
}
/**
 * Find the key and value fields of a synthetic map entry message.
 */
function findMapEntryFields(mapEntry) {
    const key = mapEntry.fields.find((f) => f.number === 1);
    const value = mapEntry.fields.find((f) => f.number === 2);
    assert(key &&
        key.fieldKind == "scalar" &&
        key.scalar != descriptors/* ScalarType */.L.BYTES &&
        key.scalar != descriptors/* ScalarType */.L.FLOAT &&
        key.scalar != descriptors/* ScalarType */.L.DOUBLE &&
        value &&
        value.fieldKind != "list" &&
        value.fieldKind != "map");
    return { key, value };
}
/**
 * Enumerations can be open or closed.
 * See https://protobuf.dev/programming-guides/enum/
 */
function isEnumOpen(desc) {
    var _a;
    return (OPEN ==
        resolveFeature("enumType", {
            proto: desc.proto,
            parent: (_a = desc.parent) !== null && _a !== void 0 ? _a : desc.file,
        }));
}
/**
 * Encode the message delimited (a.k.a. proto2 group encoding), or
 * length-prefixed?
 */
function isDelimitedEncoding(proto, parent) {
    if (proto.type == TYPE_GROUP) {
        return true;
    }
    return (DELIMITED ==
        resolveFeature("messageEncoding", {
            proto,
            parent,
        }));
}
function resolveFeature(name, ref) {
    var _a, _b;
    const featureSet = (_a = ref.proto.options) === null || _a === void 0 ? void 0 : _a.features;
    if (featureSet) {
        const val = featureSet[name];
        if (val != 0) {
            return val;
        }
    }
    if ("kind" in ref) {
        if (ref.kind == "message") {
            return resolveFeature(name, (_b = ref.parent) !== null && _b !== void 0 ? _b : ref.file);
        }
        const editionDefaults = featureDefaults[ref.edition];
        if (!editionDefaults) {
            throw new Error(`feature default for edition ${ref.edition} not found`);
        }
        return editionDefaults[name];
    }
    return resolveFeature(name, ref.parent);
}
/**
 * Assert that condition is truthy or throw error (with message)
 */
function assert(condition, msg) {
    if (!condition) {
        throw new Error(msg);
    }
}


/***/ }),

/***/ 35250:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   RM: () => (/* binding */ protoCamelCase),
/* harmony export */   aV: () => (/* binding */ safeObjectProperty),
/* harmony export */   t5: () => (/* binding */ protoSnakeCase)
/* harmony export */ });
/* unused harmony export qualifiedName */
// Copyright 2021-2026 Buf Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
/**
 * Return a fully-qualified name for a Protobuf descriptor.
 * For a file descriptor, return the original file path.
 *
 * See https://protobuf.com/docs/language-spec#fully-qualified-names
 */
function qualifiedName(desc) {
    switch (desc.kind) {
        case "field":
        case "oneof":
        case "rpc":
            return desc.parent.typeName + "." + desc.name;
        case "enum_value": {
            const p = desc.parent.parent
                ? desc.parent.parent.typeName
                : desc.parent.file.proto.package;
            return p + (p.length > 0 ? "." : "") + desc.name;
        }
        case "service":
        case "message":
        case "enum":
        case "extension":
            return desc.typeName;
        case "file":
            return desc.proto.name;
    }
}
/**
 * Converts snake_case to protoCamelCase according to the convention
 * used by protoc to convert a field name to a JSON name.
 *
 * See https://protobuf.com/docs/language-spec#default-json-names
 *
 * The function protoSnakeCase provides the reverse.
 */
function protoCamelCase(snakeCase) {
    let capNext = false;
    const b = [];
    for (let i = 0; i < snakeCase.length; i++) {
        let c = snakeCase.charAt(i);
        switch (c) {
            case "_":
                capNext = true;
                break;
            case "0":
            case "1":
            case "2":
            case "3":
            case "4":
            case "5":
            case "6":
            case "7":
            case "8":
            case "9":
                b.push(c);
                capNext = false;
                break;
            default:
                if (capNext) {
                    capNext = false;
                    c = c.toUpperCase();
                }
                b.push(c);
                break;
        }
    }
    return b.join("");
}
/**
 * Converts protoCamelCase to snake_case.
 *
 * This function is the reverse of function protoCamelCase. Note that some names
 * are not reversible - for example, "foo__bar" -> "fooBar" -> "foo_bar".
 */
function protoSnakeCase(lowerCamelCase) {
    return lowerCamelCase.replace(/[A-Z]/g, (letter) => "_" + letter.toLowerCase());
}
/**
 * Names that cannot be used for object properties because they are reserved
 * by built-in JavaScript properties.
 */
const reservedObjectProperties = new Set([
    // names reserved by JavaScript
    "constructor",
    "toString",
    "toJSON",
    "valueOf",
]);
/**
 * Escapes names that are reserved for ECMAScript built-in object properties.
 *
 * Also see safeIdentifier() from @bufbuild/protoplugin.
 */
function safeObjectProperty(name) {
    return reservedObjectProperties.has(name) ? name + "$" : name;
}


/***/ }),

/***/ 35459:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Xg: () => (/* binding */ protocolVersion)
/* harmony export */ });
/* unused harmony exports requireProtocolVersionHeader, requireProtocolVersionParam */
// Copyright 2021-2025 The Connect Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.




/**
 * The only know value for the header Connect-Protocol-Version.
 *
 * @private Internal code, does not follow semantic versioning.
 */
const protocolVersion = "1";
/**
 * Requires the Connect-Protocol-Version header to be present with the expected
 * value. Raises a ConnectError with Code.InvalidArgument otherwise.
 *
 * @private Internal code, does not follow semantic versioning.
 */
function requireProtocolVersionHeader(requestHeader) {
    const v = requestHeader.get(headerProtocolVersion);
    if (v === null) {
        throw new ConnectError(`missing required header: set ${headerProtocolVersion} to "${protocolVersion}"`, Code.InvalidArgument);
    }
    if (v !== protocolVersion) {
        throw new ConnectError(`${headerProtocolVersion} must be "${protocolVersion}": got "${v}"`, Code.InvalidArgument);
    }
}
/**
 * Requires the connect query parameter to be present with the expected value.
 * Raises a ConnectError with Code.InvalidArgument otherwise.
 *
 * @private Internal code, does not follow semantic versioning.
 */
function requireProtocolVersionParam(queryParams) {
    const v = queryParams.get(paramConnectVersion);
    if (v === null) {
        throw new ConnectError(`missing required parameter: set ${paramConnectVersion} to "v${protocolVersion}"`, Code.InvalidArgument);
    }
    if (v !== `v${protocolVersion}`) {
        throw new ConnectError(`${paramConnectVersion} must be "v${protocolVersion}": got "${v}"`, Code.InvalidArgument);
    }
}


/***/ }),

/***/ 46564:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  w: () => (/* binding */ createConnectTransport)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/from-json.js + 1 modules
var from_json = __webpack_require__(89748);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/context-values.js
var context_values = __webpack_require__(13399);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/http-headers.js
// Copyright 2021-2025 The Connect Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.




function encodeBinaryHeader(value, desc) {
    let bytes;
    if (desc !== undefined) {
        bytes = toBinary(desc, value);
    }
    else if (typeof value == "string") {
        bytes = new TextEncoder().encode(value);
    }
    else {
        bytes =
            value instanceof Uint8Array
                ? value
                : new Uint8Array(value);
    }
    return base64Encode(bytes, "std_raw");
}
function decodeBinaryHeader(value, desc, options) {
    try {
        const bytes = base64Decode(value);
        if (desc) {
            return fromBinary(desc, bytes, options);
        }
        return bytes;
    }
    catch (e) {
        throw ConnectError.from(e, Code.DataLoss);
    }
}
/**
 * Merge two or more Headers objects by appending all fields from
 * all inputs to a new Headers object.
 */
function appendHeaders(...headers) {
    const h = new Headers();
    for (const e of headers) {
        e.forEach((value, key) => {
            h.append(key, value);
        });
    }
    return h;
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/connect-error.js
var connect_error = __webpack_require__(45640);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/code.js
var code = __webpack_require__(16294);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/protocol/serialization.js + 1 modules
var serialization = __webpack_require__(70050);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/protocol/run-call.js + 3 modules
var run_call = __webpack_require__(9103);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/protocol/create-method-url.js
var create_method_url = __webpack_require__(69522);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/protocol/limit-io.js
// Copyright 2021-2025 The Connect Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.


/**
 * At most, allow ~4GiB to be received or sent per message.
 * zlib used by Node.js caps maxOutputLength at this value. It also happens to
 * be the maximum theoretical message size supported by protobuf-es.
 */
const maxReadMaxBytes = 0xffffffff;
const maxWriteMaxBytes = (/* unused pure expression or super */ null && (maxReadMaxBytes));
/**
 * The default value for the compressMinBytes option. The CPU cost of compressing
 * very small messages usually isn't worth the small reduction in network I/O, so
 * the default value is 1 kibibyte.
 */
const defaultCompressMinBytes = 1024;
/**
 * Asserts that the options writeMaxBytes, readMaxBytes, and compressMinBytes
 * are within sane limits, and returns default values where no value is
 * provided.
 *
 * @private Internal code, does not follow semantic versioning.
 */
function validateReadWriteMaxBytes(readMaxBytes, writeMaxBytes, compressMinBytes) {
    writeMaxBytes !== null && writeMaxBytes !== void 0 ? writeMaxBytes : (writeMaxBytes = maxWriteMaxBytes);
    readMaxBytes !== null && readMaxBytes !== void 0 ? readMaxBytes : (readMaxBytes = maxReadMaxBytes);
    compressMinBytes !== null && compressMinBytes !== void 0 ? compressMinBytes : (compressMinBytes = defaultCompressMinBytes);
    if (writeMaxBytes < 1 || writeMaxBytes > maxWriteMaxBytes) {
        throw new ConnectError(`writeMaxBytes ${writeMaxBytes} must be >= 1 and <= ${maxWriteMaxBytes}`, Code.Internal);
    }
    if (readMaxBytes < 1 || readMaxBytes > maxReadMaxBytes) {
        throw new ConnectError(`readMaxBytes ${readMaxBytes} must be >= 1 and <= ${maxReadMaxBytes}`, Code.Internal);
    }
    return {
        readMaxBytes,
        writeMaxBytes,
        compressMinBytes,
    };
}
/**
 * Raise an error ResourceExhausted if more than writeMaxByte are written.
 *
 * @private Internal code, does not follow semantic versioning.
 */
function assertWriteMaxBytes(writeMaxBytes, bytesWritten) {
    if (bytesWritten > writeMaxBytes) {
        throw new ConnectError(`message size ${bytesWritten} is larger than configured writeMaxBytes ${writeMaxBytes}`, Code.ResourceExhausted);
    }
}
/**
 * Raise an error ResourceExhausted if more than readMaxBytes are read.
 *
 * @private Internal code, does not follow semantic versioning.
 */
function assertReadMaxBytes(readMaxBytes, bytesRead, totalSizeKnown = false) {
    if (bytesRead > readMaxBytes) {
        let message = `message size is larger than configured readMaxBytes ${readMaxBytes}`;
        if (totalSizeKnown) {
            message = `message size ${bytesRead} is larger than configured readMaxBytes ${readMaxBytes}`;
        }
        throw new connect_error/* ConnectError */.T(message, code/* Code */.C.ResourceExhausted);
    }
}

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/protocol/envelope.js
// Copyright 2021-2025 The Connect Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.




/**
 * Create an EnvelopeDecoder. The `readMaxBytes` argument limits the maximum
 * size for individual messages.
 *
 * @private Internal code, does not follow semantic versioning.
 */
function createEnvelopeDecoder(readMaxBytes) {
    return new EnvelopeDecoderImpl(readMaxBytes);
}
class EnvelopeDecoderImpl {
    constructor(readMaxBytes) {
        this.readMaxBytes = readMaxBytes;
        // Envelope headers are 5 bytes: 1 byte for flags, 4 bytes message length
        this.header = new Uint8Array(5);
        this.headerView = new DataView(this.header.buffer);
        this.buf = [];
    }
    get byteLength() {
        return this.buf.reduce((a, b) => a + b.byteLength, 0);
    }
    decode(chunk) {
        this.buf.push(chunk);
        const envs = [];
        for (;;) {
            let env = this.pop();
            if (!env) {
                break;
            }
            envs.push(env);
        }
        return envs;
    }
    // consume an enveloped message
    pop() {
        if (!this.env) {
            this.env = this.head();
            if (!this.env) {
                return undefined;
            }
        }
        if (this.cons(this.env.data)) {
            const env = this.env;
            this.env = undefined;
            return env;
        }
        return undefined;
    }
    // consume header
    head() {
        if (!this.cons(this.header)) {
            return undefined;
        }
        const flags = this.headerView.getUint8(0); // first byte is flags
        const length = this.headerView.getUint32(1); // 4 bytes message length
        assertReadMaxBytes(this.readMaxBytes, length, true);
        return {
            flags,
            data: new Uint8Array(length),
        };
    }
    // consume from buffer, fill target
    cons(target) {
        const wantLength = target.byteLength;
        if (this.byteLength < wantLength) {
            return false;
        }
        let offset = 0;
        while (offset < wantLength) {
            const chunk = this.buf.shift(); // we check length above
            if (chunk.byteLength > wantLength - offset) {
                target.set(chunk.subarray(0, wantLength - offset), offset);
                this.buf.unshift(chunk.subarray(wantLength - offset));
                offset += wantLength - offset;
            }
            else {
                target.set(chunk, offset);
                offset += chunk.byteLength;
            }
        }
        return true;
    }
}
/**
 * Create a WHATWG ReadableStream of enveloped messages from a ReadableStream
 * of bytes.
 *
 * Ideally, this would simply be a TransformStream, but ReadableStream.pipeThrough
 * does not have the necessary availability at this time.
 *
 * @private Internal code, does not follow semantic versioning.
 */
function createEnvelopeReadableStream(stream) {
    let reader;
    const buffer = createEnvelopeDecoder(0xffffffff);
    return new ReadableStream({
        start() {
            reader = stream.getReader();
        },
        async pull(controller) {
            let enqueuedOnce = false;
            while (!enqueuedOnce) {
                const result = await reader.read();
                if (result.done) {
                    if (buffer.byteLength > 0) {
                        controller.error(new connect_error/* ConnectError */.T("protocol error: incomplete envelope", code/* Code */.C.InvalidArgument));
                    }
                    controller.close();
                }
                else {
                    for (const env of buffer.decode(result.value)) {
                        controller.enqueue(env);
                        enqueuedOnce = true;
                    }
                }
            }
        },
    });
}
/**
 * Compress an EnvelopedMessage.
 *
 * Raises Internal if an enveloped message is already compressed.
 *
 * @private Internal code, does not follow semantic versioning.
 */
async function envelopeCompress(envelope, compression, compressMinBytes) {
    let { flags, data } = envelope;
    if ((flags & compressedFlag) === compressedFlag) {
        throw new ConnectError("invalid envelope, already compressed", Code.Internal);
    }
    if (compression && data.byteLength >= compressMinBytes) {
        data = await compression.compress(data);
        flags = flags | compressedFlag;
    }
    return { data, flags };
}
/**
 * Decompress an EnvelopedMessage.
 *
 * Raises InvalidArgument if an envelope is compressed, but compression is null.
 *
 * Relies on the provided Compression to raise ResourceExhausted if the
 * *decompressed* message size is larger than readMaxBytes. If the envelope is
 * not compressed, readMaxBytes is not honored.
 *
 * @private Internal code, does not follow semantic versioning.
 */
async function envelopeDecompress(envelope, compression, readMaxBytes) {
    let { flags, data } = envelope;
    if ((flags & compressedFlag) === compressedFlag) {
        if (!compression) {
            throw new ConnectError("received compressed envelope, but do not know how to decompress", Code.Internal);
        }
        data = await compression.decompress(data, readMaxBytes);
        flags = flags ^ compressedFlag;
    }
    return { data, flags };
}
/**
 * Encode a single enveloped message.
 *
 * @private Internal code, does not follow semantic versioning.
 */
function encodeEnvelope(flags, data) {
    const bytes = new Uint8Array(data.length + 5);
    bytes.set(data, 5);
    const v = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
    v.setUint8(0, flags); // first byte is flags
    v.setUint32(1, data.length); // 4 bytes message length
    return bytes;
}
/**
 * Encode a set of enveloped messages.
 *
 * @private Internal code, does not follow semantic versioning.
 */
function encodeEnvelopes(...envelopes) {
    const len = envelopes.reduce((previousValue, currentValue) => previousValue + currentValue.data.length + 5, 0);
    const bytes = new Uint8Array(len);
    const v = new DataView(bytes.buffer);
    let offset = 0;
    for (const e of envelopes) {
        v.setUint8(offset, e.flags); // first byte is flags
        v.setUint32(offset + 1, e.data.length); // 4 bytes message length
        bytes.set(e.data, offset + 5);
        offset += e.data.length + 5;
    }
    return bytes;
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/protocol/compression.js
var compression = __webpack_require__(48210);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/protocol-connect/request-header.js
var request_header = __webpack_require__(46960);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/wire/base64-encoding.js
var base64_encoding = __webpack_require__(14097);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/protocol-connect/headers.js
var protocol_connect_headers = __webpack_require__(72101);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/protocol-connect/version.js
var version = __webpack_require__(35459);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/protocol-connect/get-request.js
// Copyright 2021-2025 The Connect Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.



const contentTypePrefix = "application/";
function encodeMessageForUrl(message, useBase64) {
    if (useBase64) {
        return (0,base64_encoding/* base64Encode */.K)(message, "url");
    }
    return encodeURIComponent(new TextDecoder().decode(message));
}
/**
 * @private Internal code, does not follow semantic versioning.
 */
function transformConnectPostToGetRequest(request, message, useBase64) {
    let query = `?connect=v${version/* protocolVersion */.Xg}`;
    const contentType = request.header.get(protocol_connect_headers/* headerContentType */.Z_);
    if ((contentType === null || contentType === void 0 ? void 0 : contentType.indexOf(contentTypePrefix)) === 0) {
        query +=
            "&encoding=" +
                encodeURIComponent(contentType.slice(contentTypePrefix.length));
    }
    const compression = request.header.get(protocol_connect_headers/* headerUnaryEncoding */.kq);
    if (compression !== null && compression !== "identity") {
        query += "&compression=" + encodeURIComponent(compression);
        // Force base64 for compressed payloads.
        useBase64 = true;
    }
    if (useBase64) {
        query += "&base64=1";
    }
    query += "&message=" + encodeMessageForUrl(message, useBase64);
    const url = request.url + query;
    const header = new Headers(request.header);
    // Omit headers that are not used for unary GET requests.
    for (const h of [
        protocol_connect_headers/* headerProtocolVersion */._U,
        protocol_connect_headers/* headerContentType */.Z_,
        protocol_connect_headers/* headerUnaryContentLength */.qs,
        protocol_connect_headers/* headerUnaryEncoding */.kq,
        protocol_connect_headers/* headerUnaryAcceptEncoding */.Eu,
    ]) {
        header.delete(h);
    }
    return Object.assign(Object.assign({}, request), { requestMethod: "GET", url,
        header });
}

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/protocol-connect/http-status.js
// Copyright 2021-2025 The Connect Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

/**
 * Determine the Connect error code for the given HTTP status code.
 * See https://connectrpc.com/docs/protocol/#http-to-error-code
 *
 * @private Internal code, does not follow semantic versioning.
 */
function codeFromHttpStatus(httpStatus) {
    switch (httpStatus) {
        case 400: // Bad Request
            return code/* Code */.C.Internal;
        case 401: // Unauthorized
            return code/* Code */.C.Unauthenticated;
        case 403: // Forbidden
            return code/* Code */.C.PermissionDenied;
        case 404: // Not Found
            return code/* Code */.C.Unimplemented;
        case 429: // Too Many Requests
            return code/* Code */.C.Unavailable;
        case 502: // Bad Gateway
            return code/* Code */.C.Unavailable;
        case 503: // Service Unavailable
            return code/* Code */.C.Unavailable;
        case 504: // Gateway Timeout
            return code/* Code */.C.Unavailable;
        default:
            return code/* Code */.C.Unknown;
    }
}
/**
 * Returns a HTTP status code for the given Connect code.
 * See https://connectrpc.com/docs/protocol#error-codes
 *
 * @private Internal code, does not follow semantic versioning.
 */
function codeToHttpStatus(code) {
    switch (code) {
        case Code.Canceled:
            return 499; // Client Closed Request
        case Code.Unknown:
            return 500; // Internal Server Error
        case Code.InvalidArgument:
            return 400; // Bad Request
        case Code.DeadlineExceeded:
            return 504; // Gateway Timeout
        case Code.NotFound:
            return 404; // Not Found
        case Code.AlreadyExists:
            return 409; // Conflict
        case Code.PermissionDenied:
            return 403; // Forbidden
        case Code.ResourceExhausted:
            return 429; // Too Many Requests
        case Code.FailedPrecondition:
            return 400; // Bad Request
        case Code.Aborted:
            return 409; // Conflict
        case Code.OutOfRange:
            return 400; // Bad Request
        case Code.Unimplemented:
            return 501; // Not Implemented
        case Code.Internal:
            return 500; // Internal Server Error
        case Code.Unavailable:
            return 503; // Service Unavailable
        case Code.DataLoss:
            return 500; // Internal Server Error
        case Code.Unauthenticated:
            return 401; // Unauthorized
        default:
            return 500; // same as CodeUnknown
    }
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/protocol-connect/content-type.js
var content_type = __webpack_require__(70525);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/protocol-connect/validate-response.js
// Copyright 2021-2025 The Connect Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.





/**
 * Validates response status and header for the Connect protocol.
 * Throws a ConnectError if the header indicates an error, or if
 * the content type is unexpected, with the following exception:
 * For unary RPCs with an HTTP error status, this returns an error
 * derived from the HTTP status instead of throwing it, giving an
 * implementation a chance to parse a Connect error from the wire.
 *
 * @private Internal code, does not follow semantic versioning.
 */
function validateResponse(methodKind, useBinaryFormat, status, headers) {
    const mimeType = headers.get(protocol_connect_headers/* headerContentType */.Z_);
    const parsedType = (0,content_type/* parseContentType */.xk)(mimeType);
    if (status !== 200) {
        const errorFromStatus = new connect_error/* ConnectError */.T(`HTTP ${status}`, codeFromHttpStatus(status), headers);
        // If parsedType is defined and it is not binary, then this is a unary JSON response
        if (methodKind == "unary" && parsedType && !parsedType.binary) {
            return { isUnaryError: true, unaryError: errorFromStatus };
        }
        throw errorFromStatus;
    }
    const allowedContentType = {
        binary: useBinaryFormat,
        stream: methodKind !== "unary",
    };
    if ((parsedType === null || parsedType === void 0 ? void 0 : parsedType.binary) !== allowedContentType.binary ||
        parsedType.stream !== allowedContentType.stream) {
        throw new connect_error/* ConnectError */.T(`unsupported content type ${mimeType}`, parsedType === undefined ? code/* Code */.C.Unknown : code/* Code */.C.Internal, headers);
    }
    return { isUnaryError: false };
}
/**
 * Validates response status and header for the Connect protocol.
 * This function is identical to validateResponse(), but also verifies
 * that a given encoding header is acceptable.
 *
 * @private
 */
function validateResponseWithCompression(methodKind, acceptCompression, useBinaryFormat, status, headers) {
    let compression;
    const encoding = headers.get(methodKind == "unary" ? headerUnaryEncoding : headerStreamEncoding);
    if (encoding != null && encoding.toLowerCase() !== "identity") {
        compression = acceptCompression.find((c) => c.name === encoding);
        if (!compression) {
            throw new ConnectError(`unsupported response encoding "${encoding}"`, Code.Internal, headers);
        }
    }
    return Object.assign({ compression }, validateResponse(methodKind, useBinaryFormat, status, headers));
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/protocol-connect/error-json.js
var error_json = __webpack_require__(96966);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/protocol-connect/trailer-mux.js
// Copyright 2021-2025 The Connect Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
/**
 * In unary RPCs, Connect transports trailing metadata as response header
 * fields, prefixed with "trailer-".
 *
 * This function demuxes headers and trailers into two separate Headers
 * objects.
 *
 * @private Internal code, does not follow semantic versioning.
 */
function trailerDemux(header) {
    const h = new Headers(), t = new Headers();
    header.forEach((value, key) => {
        if (key.toLowerCase().startsWith("trailer-")) {
            t.append(key.substring(8), value);
        }
        else {
            h.append(key, value);
        }
    });
    return [h, t];
}
/**
 * In unary RPCs, Connect transports trailing metadata as response header
 * fields, prefixed with "trailer-".
 *
 * This function muxes a header and a trailer into a single Headers object.
 *
 * @private Internal code, does not follow semantic versioning.
 */
function trailerMux(header, trailer) {
    const h = new Headers(header);
    trailer.forEach((value, key) => {
        h.append(`trailer-${key}`, value);
    });
    return h;
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/protocol-connect/end-stream.js
var end_stream = __webpack_require__(60713);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect-web@2.1.1+0149f91553ca585e/node_modules/@connectrpc/connect-web/dist/esm/assert-fetch-api.js
// Copyright 2021-2025 The Connect Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
/**
 * Asserts that the fetch API is available.
 */
function assertFetchApi() {
    try {
        new Headers();
    }
    catch (_) {
        throw new Error("connect-web requires the fetch API. Are you running on an old version of Node.js? Node.js is not supported in Connect for Web - please stay tuned for Connect for Node.");
    }
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/wkt/gen/google/protobuf/descriptor_pb.js + 1 modules
var descriptor_pb = __webpack_require__(8461);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect-web@2.1.1+0149f91553ca585e/node_modules/@connectrpc/connect-web/dist/esm/connect-transport.js
// Copyright 2021-2025 The Connect Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
var __await = (undefined && undefined.__await) || function (v) { return this instanceof __await ? (this.v = v, this) : new __await(v); }
var __asyncGenerator = (undefined && undefined.__asyncGenerator) || function (thisArg, _arguments, generator) {
    if (!Symbol.asyncIterator) throw new TypeError("Symbol.asyncIterator is not defined.");
    var g = generator.apply(thisArg, _arguments || []), i, q = [];
    return i = Object.create((typeof AsyncIterator === "function" ? AsyncIterator : Object).prototype), verb("next"), verb("throw"), verb("return", awaitReturn), i[Symbol.asyncIterator] = function () { return this; }, i;
    function awaitReturn(f) { return function (v) { return Promise.resolve(v).then(f, reject); }; }
    function verb(n, f) { if (g[n]) { i[n] = function (v) { return new Promise(function (a, b) { q.push([n, v, a, b]) > 1 || resume(n, v); }); }; if (f) i[n] = f(i[n]); } }
    function resume(n, v) { try { step(g[n](v)); } catch (e) { settle(q[0][3], e); } }
    function step(r) { r.value instanceof __await ? Promise.resolve(r.value.v).then(fulfill, reject) : settle(q[0][2], r); }
    function fulfill(value) { resume("next", value); }
    function reject(value) { resume("throw", value); }
    function settle(f, v) { if (f(v), q.shift(), q.length) resume(q[0][0], q[0][1]); }
};






const fetchOptions = {
    redirect: "error",
};
/**
 * Create a Transport for the Connect protocol, which makes unary and
 * server-streaming methods available to web browsers. It uses the fetch
 * API to make HTTP requests.
 */
function createConnectTransport(options) {
    var _a;
    assertFetchApi();
    const useBinaryFormat = (_a = options.useBinaryFormat) !== null && _a !== void 0 ? _a : false;
    return {
        async unary(method, signal, timeoutMs, header, message, contextValues) {
            const { serialize, parse } = (0,serialization/* createClientMethodSerializers */.Ef)(method, useBinaryFormat, options.jsonOptions, options.binaryOptions);
            timeoutMs =
                timeoutMs === undefined
                    ? options.defaultTimeoutMs
                    : timeoutMs <= 0
                        ? undefined
                        : timeoutMs;
            return await (0,run_call/* runUnaryCall */.L)({
                interceptors: options.interceptors,
                signal,
                timeoutMs,
                req: {
                    stream: false,
                    service: method.parent,
                    method,
                    requestMethod: "POST",
                    url: (0,create_method_url/* createMethodUrl */.j)(options.baseUrl, method),
                    header: (0,request_header/* requestHeader */.L)(method.methodKind, useBinaryFormat, timeoutMs, header, false),
                    contextValues: contextValues !== null && contextValues !== void 0 ? contextValues : (0,context_values/* createContextValues */.k)(),
                    message,
                },
                next: async (req) => {
                    var _a;
                    const useGet = options.useHttpGet === true &&
                        method.idempotency ===
                            descriptor_pb/* MethodOptions_IdempotencyLevel */.C9.NO_SIDE_EFFECTS;
                    let body = null;
                    if (useGet) {
                        req = transformConnectPostToGetRequest(req, serialize(req.message), useBinaryFormat);
                    }
                    else {
                        body = serialize(req.message);
                    }
                    const fetch = (_a = options.fetch) !== null && _a !== void 0 ? _a : globalThis.fetch;
                    const response = await fetch(req.url, Object.assign(Object.assign({}, fetchOptions), { method: req.requestMethod, headers: req.header, signal: req.signal, body }));
                    const { isUnaryError, unaryError } = validateResponse(method.methodKind, useBinaryFormat, response.status, response.headers);
                    if (isUnaryError) {
                        throw (0,error_json/* errorFromJson */.Nn)((await response.json()), appendHeaders(...trailerDemux(response.headers)), unaryError);
                    }
                    const [demuxedHeader, demuxedTrailer] = trailerDemux(response.headers);
                    return {
                        stream: false,
                        service: method.parent,
                        method,
                        header: demuxedHeader,
                        message: useBinaryFormat
                            ? parse(new Uint8Array(await response.arrayBuffer()))
                            : (0,from_json/* fromJson */.Rf)(method.output, (await response.json()), (0,serialization/* getJsonOptions */.B_)(options.jsonOptions)),
                        trailer: demuxedTrailer,
                    };
                },
            });
        },
        async stream(method, signal, timeoutMs, header, input, contextValues) {
            const { serialize, parse } = (0,serialization/* createClientMethodSerializers */.Ef)(method, useBinaryFormat, options.jsonOptions, options.binaryOptions);
            function parseResponseBody(body, trailerTarget, header, signal) {
                return __asyncGenerator(this, arguments, function* parseResponseBody_1() {
                    const reader = createEnvelopeReadableStream(body).getReader();
                    let endStreamReceived = false;
                    for (;;) {
                        const result = yield __await(reader.read());
                        if (result.done) {
                            break;
                        }
                        const { flags, data } = result.value;
                        if ((flags & compression/* compressedFlag */.y) === compression/* compressedFlag */.y) {
                            throw new connect_error/* ConnectError */.T(`protocol error: received unsupported compressed output`, code/* Code */.C.Internal);
                        }
                        if ((flags & end_stream/* endStreamFlag */.Cu) === end_stream/* endStreamFlag */.Cu) {
                            endStreamReceived = true;
                            const endStream = (0,end_stream/* endStreamFromJson */.mo)(data);
                            if (endStream.error) {
                                const error = endStream.error;
                                header.forEach((value, key) => {
                                    error.metadata.append(key, value);
                                });
                                throw error;
                            }
                            endStream.metadata.forEach((value, key) => trailerTarget.set(key, value));
                            continue;
                        }
                        yield yield __await(parse(data));
                    }
                    // Node wil not throw an AbortError on `read` if the
                    // signal is aborted before `getReader` is called.
                    // As a work around we check at the end and throw.
                    //
                    // Ref: https://github.com/nodejs/undici/issues/1940
                    if ("throwIfAborted" in signal) {
                        // We assume that implementations without `throwIfAborted` (old
                        // browsers) do honor aborted signals on `read`.
                        signal.throwIfAborted();
                    }
                    if (!endStreamReceived) {
                        throw "missing EndStreamResponse";
                    }
                });
            }
            async function createRequestBody(input) {
                if (method.methodKind != "server_streaming") {
                    throw "The fetch API does not support streaming request bodies";
                }
                const r = await input[Symbol.asyncIterator]().next();
                if (r.done == true) {
                    throw "missing request message";
                }
                return encodeEnvelope(0, serialize(r.value));
            }
            timeoutMs =
                timeoutMs === undefined
                    ? options.defaultTimeoutMs
                    : timeoutMs <= 0
                        ? undefined
                        : timeoutMs;
            return await (0,run_call/* runStreamingCall */.u)({
                interceptors: options.interceptors,
                timeoutMs,
                signal,
                req: {
                    stream: true,
                    service: method.parent,
                    method,
                    requestMethod: "POST",
                    url: (0,create_method_url/* createMethodUrl */.j)(options.baseUrl, method),
                    header: (0,request_header/* requestHeader */.L)(method.methodKind, useBinaryFormat, timeoutMs, header, false),
                    contextValues: contextValues !== null && contextValues !== void 0 ? contextValues : (0,context_values/* createContextValues */.k)(),
                    message: input,
                },
                next: async (req) => {
                    var _a;
                    const fetch = (_a = options.fetch) !== null && _a !== void 0 ? _a : globalThis.fetch;
                    const fRes = await fetch(req.url, Object.assign(Object.assign({}, fetchOptions), { method: req.requestMethod, headers: req.header, signal: req.signal, body: await createRequestBody(req.message) }));
                    validateResponse(method.methodKind, useBinaryFormat, fRes.status, fRes.headers);
                    if (fRes.body === null) {
                        throw "missing response body";
                    }
                    const trailer = new Headers();
                    const res = Object.assign(Object.assign({}, req), { header: fRes.headers, trailer, message: parseResponseBody(fRes.body, trailer, fRes.headers, req.signal) });
                    return res;
                },
            });
        },
    };
}


/***/ }),

/***/ 46960:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   L: () => (/* binding */ requestHeader)
/* harmony export */ });
/* unused harmony export requestHeaderWithCompression */
/* harmony import */ var _headers_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(72101);
/* harmony import */ var _version_js__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(35459);
/* harmony import */ var _content_type_js__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(70525);
// Copyright 2021-2025 The Connect Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.



/**
 * Creates headers for a Connect request.
 *
 * @private Internal code, does not follow semantic versioning.
 */
function requestHeader(methodKind, useBinaryFormat, timeoutMs, userProvidedHeaders, setUserAgent) {
    const result = new Headers(userProvidedHeaders !== null && userProvidedHeaders !== void 0 ? userProvidedHeaders : {});
    if (timeoutMs !== undefined) {
        result.set(_headers_js__WEBPACK_IMPORTED_MODULE_0__/* .headerTimeout */ .hR, `${timeoutMs}`);
    }
    result.set(_headers_js__WEBPACK_IMPORTED_MODULE_0__/* .headerContentType */ .Z_, methodKind == "unary"
        ? useBinaryFormat
            ? _content_type_js__WEBPACK_IMPORTED_MODULE_1__/* .contentTypeUnaryProto */ .Vs
            : _content_type_js__WEBPACK_IMPORTED_MODULE_1__/* .contentTypeUnaryJson */ .Br
        : useBinaryFormat
            ? _content_type_js__WEBPACK_IMPORTED_MODULE_1__/* .contentTypeStreamProto */ .Sg
            : _content_type_js__WEBPACK_IMPORTED_MODULE_1__/* .contentTypeStreamJson */ .yU);
    result.set(_headers_js__WEBPACK_IMPORTED_MODULE_0__/* .headerProtocolVersion */ ._U, _version_js__WEBPACK_IMPORTED_MODULE_2__/* .protocolVersion */ .Xg);
    if (!result.has(_headers_js__WEBPACK_IMPORTED_MODULE_0__/* .headerUserAgent */ .qy) && setUserAgent) {
        // Note that we do not strictly comply with gRPC user agents.
        // We use "connect-es/1.2.3" where gRPC would use "grpc-es/1.2.3".
        // See https://github.com/grpc/grpc/blob/c462bb8d485fc1434ecfae438823ca8d14cf3154/doc/PROTOCOL-HTTP2.md#user-agents
        result.set(_headers_js__WEBPACK_IMPORTED_MODULE_0__/* .headerUserAgent */ .qy, "connect-es/2.1.1");
    }
    return result;
}
/**
 * Creates headers for a Connect request with compression.
 *
 * Note that we always set the Content-Encoding header for unary methods.
 * It is up to the caller to decide whether to apply compression - and remove
 * the header if compression is not used, for example because the payload is
 * too small to make compression effective.
 *
 * @private Internal code, does not follow semantic versioning.
 */
function requestHeaderWithCompression(methodKind, useBinaryFormat, timeoutMs, userProvidedHeaders, acceptCompression, sendCompression, setUserAgent) {
    const result = requestHeader(methodKind, useBinaryFormat, timeoutMs, userProvidedHeaders, setUserAgent);
    if (sendCompression != null) {
        const name = methodKind == "unary" ? headerUnaryEncoding : headerStreamEncoding;
        result.set(name, sendCompression.name);
    }
    if (acceptCompression.length > 0) {
        const name = methodKind == "unary"
            ? headerUnaryAcceptEncoding
            : headerStreamAcceptEncoding;
        result.set(name, acceptCompression.map((c) => c.name).join(","));
    }
    return result;
}


/***/ }),

/***/ 48210:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   y: () => (/* binding */ compressedFlag)
/* harmony export */ });
/* unused harmony export compressionNegotiate */
// Copyright 2021-2025 The Connect Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.


/**
 * compressedFlag indicates that the data in a EnvelopedMessage is
 * compressed. It has the same meaning in the gRPC-Web, gRPC-HTTP2,
 * and Connect protocols.
 *
 * @private Internal code, does not follow semantic versioning.
 */
const compressedFlag = 0b00000001;
/**
 * Validates the request encoding and determines the accepted response encoding.
 *
 * Returns the request and response compression to use. If the client requested
 * an encoding that is not available, the returned object contains an error that
 * must be used for the response.
 *
 * @private Internal code, does not follow semantic versioning.
 */
function compressionNegotiate(available, requested, // e.g. the value of the Grpc-Encoding header
accepted, // e.g. the value of the Grpc-Accept-Encoding header
headerNameAcceptEncoding) {
    let request = null;
    let response = null;
    let error = undefined;
    if (requested !== null && requested !== "identity") {
        const found = available.find((c) => c.name === requested);
        if (found) {
            request = found;
        }
        else {
            // To comply with https://github.com/grpc/grpc/blob/master/doc/compression.md
            // and the Connect protocol, we return code "unimplemented" and specify
            // acceptable compression(s).
            const acceptable = available.map((c) => c.name).join(",");
            error = new ConnectError(`unknown compression "${requested}": supported encodings are ${acceptable}`, Code.Unimplemented, {
                [headerNameAcceptEncoding]: acceptable,
            });
        }
    }
    if (accepted === null || accepted === "") {
        // Support asymmetric compression. This logic follows
        // https://github.com/grpc/grpc/blob/master/doc/compression.md and common
        // sense.
        response = request;
    }
    else {
        const acceptNames = accepted.split(",").map((n) => n.trim());
        for (const name of acceptNames) {
            const found = available.find((c) => c.name === name);
            if (found) {
                response = found;
                break;
            }
        }
    }
    return { request, response, error };
}


/***/ }),

/***/ 60713:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Cu: () => (/* binding */ endStreamFlag),
/* harmony export */   mo: () => (/* binding */ endStreamFromJson)
/* harmony export */ });
/* unused harmony exports endStreamToJson, createEndStreamSerialization */
/* harmony import */ var _error_json_js__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(96966);
/* harmony import */ var _connect_error_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(45640);
/* harmony import */ var _code_js__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(16294);
// Copyright 2021-2025 The Connect Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.




/**
 * endStreamFlag indicates that the data in a EnvelopedMessage
 * is a EndStreamResponse of the Connect protocol.
 *
 * @private Internal code, does not follow semantic versioning.
 */
const endStreamFlag = 0b00000010;
/**
 * Parse an EndStreamResponse of the Connect protocol.
 * Throws a ConnectError on malformed input.
 *
 * @private Internal code, does not follow semantic versioning.
 */
function endStreamFromJson(data) {
    const parseErr = new _connect_error_js__WEBPACK_IMPORTED_MODULE_0__/* .ConnectError */ .T("invalid end stream", _code_js__WEBPACK_IMPORTED_MODULE_1__/* .Code */ .C.Unknown);
    let jsonValue;
    try {
        // eslint-disable-next-line @typescript-eslint/no-unsafe-assignment
        jsonValue = JSON.parse(typeof data == "string" ? data : new TextDecoder().decode(data));
    }
    catch (e) {
        throw parseErr;
    }
    if (typeof jsonValue != "object" ||
        jsonValue == null ||
        Array.isArray(jsonValue)) {
        throw parseErr;
    }
    const metadata = new Headers();
    if ("metadata" in jsonValue) {
        if (typeof jsonValue.metadata != "object" ||
            jsonValue.metadata == null ||
            Array.isArray(jsonValue.metadata)) {
            throw parseErr;
        }
        for (const [key, values] of Object.entries(jsonValue.metadata)) {
            if (!Array.isArray(values) ||
                values.some((value) => typeof value != "string")) {
                throw parseErr;
            }
            for (const value of values) {
                metadata.append(key, value);
            }
        }
    }
    const error = "error" in jsonValue && jsonValue.error != null
        ? (0,_error_json_js__WEBPACK_IMPORTED_MODULE_2__/* .errorFromJson */ .Nn)(jsonValue.error, metadata, parseErr)
        : undefined;
    return { metadata, error };
}
/**
 * Serialize the given EndStreamResponse to JSON.
 *
 * The JSON serialization options are required to produce the optional
 * human-readable representation of error details if the detail uses
 * google.protobuf.Any.
 *
 * See https://connectrpc.com/docs/protocol#error-end-stream
 *
 * @private Internal code, does not follow semantic versioning.
 */
function endStreamToJson(metadata, error, jsonWriteOptions) {
    const es = {};
    if (error !== undefined) {
        es.error = errorToJson(error, jsonWriteOptions);
        metadata = appendHeaders(metadata, error.metadata);
    }
    let hasMetadata = false;
    const md = {};
    metadata.forEach((value, key) => {
        hasMetadata = true;
        md[key] = [value];
    });
    // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition
    if (hasMetadata) {
        es.metadata = md;
    }
    return es;
}
/**
 * Create a Serialization object that serializes a Connect EndStreamResponse.
 *
 * @private Internal code, does not follow semantic versioning.
 */
function createEndStreamSerialization(options) {
    const textEncoder = new TextEncoder();
    return {
        serialize(data) {
            try {
                const jsonObject = endStreamToJson(data.metadata, data.error, options);
                const jsonString = JSON.stringify(jsonObject);
                return textEncoder.encode(jsonString);
            }
            catch (e) {
                const m = e instanceof Error ? e.message : String(e);
                throw new ConnectError(`failed to serialize EndStreamResponse: ${m}`, Code.Internal);
            }
        },
        parse(data) {
            try {
                return endStreamFromJson(data);
            }
            catch (e) {
                const m = e instanceof Error ? e.message : String(e);
                throw new ConnectError(`failed to parse EndStreamResponse: ${m}`, Code.InvalidArgument);
            }
        },
    };
}


/***/ }),

/***/ 65402:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   G6: () => (/* binding */ setExtension),
/* harmony export */   uQ: () => (/* binding */ getExtension),
/* harmony export */   z3: () => (/* binding */ createExtensionContainer)
/* harmony export */ });
/* unused harmony exports clearExtension, hasExtension, hasOption, getOption */
/* harmony import */ var _create_js__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(95444);
/* harmony import */ var _from_binary_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(57386);
/* harmony import */ var _reflect_reflect_js__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(64683);
/* harmony import */ var _reflect_scalar_js__WEBPACK_IMPORTED_MODULE_6__ = __webpack_require__(17134);
/* harmony import */ var _to_binary_js__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(2681);
/* harmony import */ var _wire_binary_encoding_js__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(26811);
/* harmony import */ var _wkt_wrappers_js__WEBPACK_IMPORTED_MODULE_5__ = __webpack_require__(34123);
// Copyright 2021-2026 Buf Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.







/**
 * Retrieve an extension value from a message.
 *
 * The function never returns undefined. Use hasExtension() to check whether an
 * extension is set. If the extension is not set, this function returns the
 * default value (if one was specified in the protobuf source), or the zero value
 * (for example `0` for numeric types, `[]` for repeated extension fields, and
 * an empty message instance for message fields).
 *
 * Extensions are stored as unknown fields on a message. To mutate an extension
 * value, make sure to store the new value with setExtension() after mutating.
 *
 * If the extension does not extend the given message, an error is raised.
 */
function getExtension(message, extension) {
    assertExtendee(extension, message);
    const ufs = filterUnknownFields(message.$unknown, extension);
    const [container, field, get] = createExtensionContainer(extension);
    for (const uf of ufs) {
        (0,_from_binary_js__WEBPACK_IMPORTED_MODULE_0__/* .readField */ .dR)(container, new _wire_binary_encoding_js__WEBPACK_IMPORTED_MODULE_1__/* .BinaryReader */ .V5(uf.data), field, uf.wireType, {
            readUnknownFields: true,
        });
    }
    return get();
}
/**
 * Set an extension value on a message. If the message already has a value for
 * this extension, the value is replaced.
 *
 * If the extension does not extend the given message, an error is raised.
 */
function setExtension(message, extension, value) {
    var _a;
    assertExtendee(extension, message);
    const ufs = ((_a = message.$unknown) !== null && _a !== void 0 ? _a : []).filter((uf) => uf.no !== extension.number);
    const [container, field] = createExtensionContainer(extension, value);
    const writer = new _wire_binary_encoding_js__WEBPACK_IMPORTED_MODULE_1__/* .BinaryWriter */ .fI();
    (0,_to_binary_js__WEBPACK_IMPORTED_MODULE_2__/* .writeField */ .Y)(writer, { writeUnknownFields: true }, container, field);
    const reader = new _wire_binary_encoding_js__WEBPACK_IMPORTED_MODULE_1__/* .BinaryReader */ .V5(writer.finish());
    while (reader.pos < reader.len) {
        const [no, wireType] = reader.tag();
        const data = reader.skip(wireType, no);
        ufs.push({ no, wireType, data });
    }
    message.$unknown = ufs;
}
/**
 * Remove an extension value from a message.
 *
 * If the extension does not extend the given message, an error is raised.
 */
function clearExtension(message, extension) {
    assertExtendee(extension, message);
    if (message.$unknown === undefined) {
        return;
    }
    message.$unknown = message.$unknown.filter((uf) => uf.no !== extension.number);
}
/**
 * Check whether an extension is set on a message.
 */
function hasExtension(message, extension) {
    var _a;
    return (extension.extendee.typeName === message.$typeName &&
        !!((_a = message.$unknown) === null || _a === void 0 ? void 0 : _a.find((uf) => uf.no === extension.number)));
}
/**
 * Check whether an option is set on a descriptor.
 *
 * Options are extensions to the `google.protobuf.*Options` messages defined in
 * google/protobuf/descriptor.proto. This function gets the option message from
 * the descriptor, and calls hasExtension().
 */
function hasOption(element, option) {
    const message = element.proto.options;
    if (!message) {
        return false;
    }
    return hasExtension(message, option);
}
/**
 * Retrieve an option value from a descriptor.
 *
 * Options are extensions to the `google.protobuf.*Options` messages defined in
 * google/protobuf/descriptor.proto. This function gets the option message from
 * the descriptor, and calls getExtension(). Same as getExtension(), this
 * function never returns undefined.
 */
function getOption(element, option) {
    const message = element.proto.options;
    if (!message) {
        const [, , get] = createExtensionContainer(option);
        return get();
    }
    return getExtension(message, option);
}
function filterUnknownFields(unknownFields, extension) {
    if (unknownFields === undefined)
        return [];
    if (extension.fieldKind === "enum" || extension.fieldKind === "scalar") {
        // singular scalar fields do not merge, we pick the last
        for (let i = unknownFields.length - 1; i >= 0; --i) {
            if (unknownFields[i].no == extension.number) {
                return [unknownFields[i]];
            }
        }
        return [];
    }
    return unknownFields.filter((uf) => uf.no === extension.number);
}
/**
 * @private
 */
function createExtensionContainer(extension, value) {
    const localName = extension.typeName;
    const field = Object.assign(Object.assign({}, extension), { kind: "field", parent: extension.extendee, localName });
    const desc = Object.assign(Object.assign({}, extension.extendee), { fields: [field], members: [field], oneofs: [] });
    const container = (0,_create_js__WEBPACK_IMPORTED_MODULE_3__/* .create */ .v)(desc, value !== undefined ? { [localName]: value } : undefined);
    return [
        (0,_reflect_reflect_js__WEBPACK_IMPORTED_MODULE_4__/* .reflect */ .E4)(desc, container),
        field,
        () => {
            const value = container[localName];
            if (value === undefined) {
                // biome-ignore lint/style/noNonNullAssertion: Only message fields are undefined, rest will have a zero value.
                const desc = extension.message;
                if ((0,_wkt_wrappers_js__WEBPACK_IMPORTED_MODULE_5__/* .isWrapperDesc */ .l)(desc)) {
                    return (0,_reflect_scalar_js__WEBPACK_IMPORTED_MODULE_6__/* .scalarZeroValue */ .M7)(desc.fields[0].scalar, desc.fields[0].longAsString);
                }
                return (0,_create_js__WEBPACK_IMPORTED_MODULE_3__/* .create */ .v)(desc);
            }
            return value;
        },
    ];
}
function assertExtendee(extension, message) {
    if (extension.extendee.typeName != message.$typeName) {
        throw new Error(`extension ${extension.typeName} can only be applied to message ${extension.extendee.typeName}`);
    }
}


/***/ }),

/***/ 66144:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  UU: () => (/* binding */ createClient)
});

// UNUSED EXPORTS: createBiDiStreamingFn, createClientStreamingFn, createServerStreamingFn, createUnaryFn

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/any-client.js
// Copyright 2021-2025 The Connect Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
/**
 * Create any client for the given service.
 *
 * The given createMethod function is called for each method definition
 * of the service. The function it returns is added to the client object
 * as a method.
 */
function makeAnyClient(service, createMethod) {
    const client = {};
    for (const desc of service.methods) {
        const method = createMethod(desc);
        if (method != null) {
            client[desc.localName] = method;
        }
    }
    return client;
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/connect-error.js
var connect_error = __webpack_require__(45640);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/code.js
var code = __webpack_require__(16294);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/protocol/async-iterable.js
// Copyright 2021-2025 The Connect Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
var __asyncValues = (undefined && undefined.__asyncValues) || function (o) {
    if (!Symbol.asyncIterator) throw new TypeError("Symbol.asyncIterator is not defined.");
    var m = o[Symbol.asyncIterator], i;
    return m ? m.call(o) : (o = typeof __values === "function" ? __values(o) : o[Symbol.iterator](), i = {}, verb("next"), verb("throw"), verb("return"), i[Symbol.asyncIterator] = function () { return this; }, i);
    function verb(n) { i[n] = o[n] && function (v) { return new Promise(function (resolve, reject) { v = o[n](v), settle(resolve, reject, v.done, v.value); }); }; }
    function settle(resolve, reject, d, v) { Promise.resolve(v).then(function(v) { resolve({ value: v, done: d }); }, reject); }
};
var __await = (undefined && undefined.__await) || function (v) { return this instanceof __await ? (this.v = v, this) : new __await(v); }
var __asyncGenerator = (undefined && undefined.__asyncGenerator) || function (thisArg, _arguments, generator) {
    if (!Symbol.asyncIterator) throw new TypeError("Symbol.asyncIterator is not defined.");
    var g = generator.apply(thisArg, _arguments || []), i, q = [];
    return i = Object.create((typeof AsyncIterator === "function" ? AsyncIterator : Object).prototype), verb("next"), verb("throw"), verb("return", awaitReturn), i[Symbol.asyncIterator] = function () { return this; }, i;
    function awaitReturn(f) { return function (v) { return Promise.resolve(v).then(f, reject); }; }
    function verb(n, f) { if (g[n]) { i[n] = function (v) { return new Promise(function (a, b) { q.push([n, v, a, b]) > 1 || resume(n, v); }); }; if (f) i[n] = f(i[n]); } }
    function resume(n, v) { try { step(g[n](v)); } catch (e) { settle(q[0][3], e); } }
    function step(r) { r.value instanceof __await ? Promise.resolve(r.value.v).then(fulfill, reject) : settle(q[0][2], r); }
    function fulfill(value) { resume("next", value); }
    function reject(value) { resume("throw", value); }
    function settle(f, v) { if (f(v), q.shift(), q.length) resume(q[0][0], q[0][1]); }
};
var __asyncDelegator = (undefined && undefined.__asyncDelegator) || function (o) {
    var i, p;
    return i = {}, verb("next"), verb("throw", function (e) { throw e; }), verb("return"), i[Symbol.iterator] = function () { return this; }, i;
    function verb(n, f) { i[n] = o[n] ? function (v) { return (p = !p) ? { value: __await(o[n](v)), done: false } : f ? f(v) : v; } : f; }
};





function pipeTo(source, ...rest) {
    const [transforms, sink, opt] = pickTransformsAndSink(rest);
    let iterable = source;
    let abortable;
    if ((opt === null || opt === void 0 ? void 0 : opt.propagateDownStreamError) === true) {
        iterable = abortable = makeIterableAbortable(iterable);
    }
    // @ts-ignore
    iterable = pipe(iterable, ...transforms, { propagateDownStreamError: false });
    return sink(iterable).catch((reason) => {
        if (abortable) {
            return abortable.abort(reason).then(() => Promise.reject(reason));
        }
        return Promise.reject(reason);
    });
}
// pick transforms, the sink, and options from the pipeTo() rest parameter
function pickTransformsAndSink(rest) {
    let opt;
    if (typeof rest[rest.length - 1] != "function") {
        opt = rest.pop();
    }
    const sink = rest.pop();
    return [rest, sink, opt];
}
/**
 * Creates an AsyncIterableSink that concatenates all elements from the input.
 *
 * @private Internal code, does not follow semantic versioning.
 */
function sinkAll() {
    return async (iterable) => {
        var _a, e_1, _b, _c;
        const all = [];
        try {
            for (var _d = true, iterable_1 = __asyncValues(iterable), iterable_1_1; iterable_1_1 = await iterable_1.next(), _a = iterable_1_1.done, !_a; _d = true) {
                _c = iterable_1_1.value;
                _d = false;
                const chunk = _c;
                all.push(chunk);
            }
        }
        catch (e_1_1) { e_1 = { error: e_1_1 }; }
        finally {
            try {
                if (!_d && !_a && (_b = iterable_1.return)) await _b.call(iterable_1);
            }
            finally { if (e_1) throw e_1.error; }
        }
        return all;
    };
}
/**
 * Creates an AsyncIterableSink that concatenates all chunks from the input into
 * a single Uint8Array.
 *
 * The iterable raises an error if the more than readMaxBytes are read.
 *
 * An optional length hint can be provided to optimize allocation and validation.
 * If more or less bytes are present in the source that the length hint indicates,
 * and error is raised.
 * If the length hint is larger than readMaxBytes, an error is raised.
 * If the length hint is not a positive integer, it is ignored.
 *
 * @private Internal code, does not follow semantic versioning.
 */
function sinkAllBytes(readMaxBytes, lengthHint) {
    return async (iterable) => await readAllBytes(iterable, readMaxBytes, lengthHint);
}
function pipe(source, ...rest) {
    return __asyncGenerator(this, arguments, function* pipe_1() {
        var _a;
        const [transforms, opt] = pickTransforms(rest);
        let abortable;
        const sourceIt = source[Symbol.asyncIterator]();
        const cachedSource = {
            [Symbol.asyncIterator]() {
                return sourceIt;
            },
        };
        let iterable = cachedSource;
        if ((opt === null || opt === void 0 ? void 0 : opt.propagateDownStreamError) === true) {
            iterable = abortable = makeIterableAbortable(iterable);
        }
        for (const t of transforms) {
            iterable = t(iterable);
        }
        const it = iterable[Symbol.asyncIterator]();
        try {
            for (;;) {
                const r = yield __await(it.next());
                if (r.done === true) {
                    break;
                }
                if (!abortable) {
                    yield yield __await(r.value);
                    continue;
                }
                try {
                    yield yield __await(r.value);
                }
                catch (e) {
                    yield __await(abortable.abort(e)); // propagate downstream error to the source
                    throw e;
                }
            }
        }
        finally {
            if ((opt === null || opt === void 0 ? void 0 : opt.propagateDownStreamError) === true) {
                // Call return on the source iterable to indicate
                // that we will no longer consume it and it should
                // cleanup any allocated resources.
                (_a = sourceIt.return) === null || _a === void 0 ? void 0 : _a.call(sourceIt).catch(() => {
                    // return returns a promise, which we don't care about.
                    //
                    // Uncaught promises are thrown at sometime/somewhere by the event loop,
                    // this is to ensure error is caught and ignored.
                });
            }
        }
    });
}
function pickTransforms(rest) {
    let opt;
    if (typeof rest[rest.length - 1] != "function") {
        opt = rest.pop();
    }
    return [rest, opt];
}
/**
 * Creates an AsyncIterableTransform that catches any error from the input, and
 * passes it to the given catchError function.
 *
 * The catchError function may return a final value.
 *
 * @private Internal code, does not follow semantic versioning.
 */
function transformCatch(catchError) {
    return function (iterable) {
        return __asyncGenerator(this, arguments, function* () {
            // we deliberate avoid a for-await loop because we only want to catch upstream
            // errors, not downstream errors (yield).
            const it = iterable[Symbol.asyncIterator]();
            for (;;) {
                let r;
                try {
                    r = yield __await(it.next());
                }
                catch (e) {
                    const caught = yield __await(catchError(e));
                    if (caught !== undefined) {
                        yield yield __await(caught);
                    }
                    break;
                }
                if (r.done === true) {
                    break;
                }
                yield yield __await(r.value);
            }
        });
    };
}
/**
 * Creates an AsyncIterableTransform that catches any error from the input, and
 * passes it to the given function. Unlike transformCatch(), the given function
 * is also called when no error is raised.
 *
 * @private Internal code, does not follow semantic versioning.
 */
function transformCatchFinally(catchFinally) {
    return function (iterable) {
        return __asyncGenerator(this, arguments, function* () {
            // we deliberate avoid a for-await loop because we only want to catch upstream
            // errors, not downstream errors (yield).
            let err;
            const it = iterable[Symbol.asyncIterator]();
            for (;;) {
                let r;
                try {
                    r = yield __await(it.next());
                }
                catch (e) {
                    err = e;
                    break;
                }
                if (r.done === true) {
                    break;
                }
                yield yield __await(r.value);
            }
            const caught = yield __await(catchFinally(err));
            if (caught !== undefined) {
                yield yield __await(caught);
            }
        });
    };
}
/**
 * Creates an AsyncIterableTransform that appends a value.
 *
 * The element to append is provided by a function. If the function returns
 * undefined, no element is appended.
 *
 * @private Internal code, does not follow semantic versioning.
 */
function transformAppend(provide) {
    return function (iterable) {
        return __asyncGenerator(this, arguments, function* () {
            var _a, e_2, _b, _c;
            try {
                for (var _d = true, iterable_2 = __asyncValues(iterable), iterable_2_1; iterable_2_1 = yield __await(iterable_2.next()), _a = iterable_2_1.done, !_a; _d = true) {
                    _c = iterable_2_1.value;
                    _d = false;
                    const chunk = _c;
                    yield yield __await(chunk);
                }
            }
            catch (e_2_1) { e_2 = { error: e_2_1 }; }
            finally {
                try {
                    if (!_d && !_a && (_b = iterable_2.return)) yield __await(_b.call(iterable_2));
                }
                finally { if (e_2) throw e_2.error; }
            }
            const append = yield __await(provide());
            if (append !== undefined) {
                yield yield __await(append);
            }
        });
    };
}
/**
 * Creates an AsyncIterableTransform that prepends an element.
 *
 * The element to prepend is provided by a function. If the function returns
 * undefined, no element is appended.
 *
 * @private Internal code, does not follow semantic versioning.
 */
function transformPrepend(provide) {
    return function (iterable) {
        return __asyncGenerator(this, arguments, function* () {
            var _a, e_3, _b, _c;
            const prepend = yield __await(provide());
            if (prepend !== undefined) {
                yield yield __await(prepend);
            }
            try {
                for (var _d = true, iterable_3 = __asyncValues(iterable), iterable_3_1; iterable_3_1 = yield __await(iterable_3.next()), _a = iterable_3_1.done, !_a; _d = true) {
                    _c = iterable_3_1.value;
                    _d = false;
                    const chunk = _c;
                    yield yield __await(chunk);
                }
            }
            catch (e_3_1) { e_3 = { error: e_3_1 }; }
            finally {
                try {
                    if (!_d && !_a && (_b = iterable_3.return)) yield __await(_b.call(iterable_3));
                }
                finally { if (e_3) throw e_3.error; }
            }
        });
    };
}
/**
 * Creates an AsyncIterableTransform that reads all bytes from the input, and
 * concatenates them to a single Uint8Array.
 *
 * The iterable raises an error if the more than readMaxBytes are read.
 *
 * An optional length hint can be provided to optimize allocation and validation.
 * If more or less bytes are present in the source that the length hint indicates,
 * and error is raised.
 * If the length hint is larger than readMaxBytes, an error is raised.
 * If the length hint is not a positive integer, it is ignored.
 *
 * @private Internal code, does not follow semantic versioning.
 */
function transformReadAllBytes(readMaxBytes, lengthHint) {
    return function (iterable) {
        return __asyncGenerator(this, arguments, function* () {
            yield yield __await(yield __await(readAllBytes(iterable, readMaxBytes, lengthHint)));
        });
    };
}
function transformSerializeEnvelope(serialization, endStreamFlag, endSerialization) {
    if (endStreamFlag === undefined || endSerialization === undefined) {
        return function (iterable) {
            return __asyncGenerator(this, arguments, function* () {
                var _a, e_4, _b, _c;
                try {
                    for (var _d = true, iterable_4 = __asyncValues(iterable), iterable_4_1; iterable_4_1 = yield __await(iterable_4.next()), _a = iterable_4_1.done, !_a; _d = true) {
                        _c = iterable_4_1.value;
                        _d = false;
                        const chunk = _c;
                        const data = serialization.serialize(chunk);
                        yield yield __await({ flags: 0, data });
                    }
                }
                catch (e_4_1) { e_4 = { error: e_4_1 }; }
                finally {
                    try {
                        if (!_d && !_a && (_b = iterable_4.return)) yield __await(_b.call(iterable_4));
                    }
                    finally { if (e_4) throw e_4.error; }
                }
            });
        };
    }
    return function (iterable) {
        return __asyncGenerator(this, arguments, function* () {
            var _a, e_5, _b, _c;
            try {
                for (var _d = true, iterable_5 = __asyncValues(iterable), iterable_5_1; iterable_5_1 = yield __await(iterable_5.next()), _a = iterable_5_1.done, !_a; _d = true) {
                    _c = iterable_5_1.value;
                    _d = false;
                    const chunk = _c;
                    let data;
                    let flags = 0;
                    if (chunk.end) {
                        flags = flags | endStreamFlag;
                        data = endSerialization.serialize(chunk.value);
                    }
                    else {
                        data = serialization.serialize(chunk.value);
                    }
                    yield yield __await({ flags, data });
                }
            }
            catch (e_5_1) { e_5 = { error: e_5_1 }; }
            finally {
                try {
                    if (!_d && !_a && (_b = iterable_5.return)) yield __await(_b.call(iterable_5));
                }
                finally { if (e_5) throw e_5.error; }
            }
        });
    };
}
function transformParseEnvelope(serialization, endStreamFlag, endSerialization) {
    // code path always yields ParsedEnvelopedMessage<T, E>
    if (endSerialization && endStreamFlag !== undefined) {
        return function (iterable) {
            return __asyncGenerator(this, arguments, function* () {
                var _a, e_6, _b, _c;
                try {
                    for (var _d = true, iterable_6 = __asyncValues(iterable), iterable_6_1; iterable_6_1 = yield __await(iterable_6.next()), _a = iterable_6_1.done, !_a; _d = true) {
                        _c = iterable_6_1.value;
                        _d = false;
                        const { flags, data } = _c;
                        if ((flags & endStreamFlag) === endStreamFlag) {
                            yield yield __await({ value: endSerialization.parse(data), end: true });
                        }
                        else {
                            yield yield __await({ value: serialization.parse(data), end: false });
                        }
                    }
                }
                catch (e_6_1) { e_6 = { error: e_6_1 }; }
                finally {
                    try {
                        if (!_d && !_a && (_b = iterable_6.return)) yield __await(_b.call(iterable_6));
                    }
                    finally { if (e_6) throw e_6.error; }
                }
            });
        };
    }
    // code path always yields T
    return function (iterable) {
        return __asyncGenerator(this, arguments, function* () {
            var _a, e_7, _b, _c;
            try {
                for (var _d = true, iterable_7 = __asyncValues(iterable), iterable_7_1; iterable_7_1 = yield __await(iterable_7.next()), _a = iterable_7_1.done, !_a; _d = true) {
                    _c = iterable_7_1.value;
                    _d = false;
                    const { flags, data } = _c;
                    if (endStreamFlag !== undefined &&
                        (flags & endStreamFlag) === endStreamFlag) {
                        if (endSerialization === null) {
                            throw new ConnectError("unexpected end flag", Code.InvalidArgument);
                        }
                        // skips end-of-stream envelope
                        continue;
                    }
                    yield yield __await(serialization.parse(data));
                }
            }
            catch (e_7_1) { e_7 = { error: e_7_1 }; }
            finally {
                try {
                    if (!_d && !_a && (_b = iterable_7.return)) yield __await(_b.call(iterable_7));
                }
                finally { if (e_7) throw e_7.error; }
            }
        });
    };
}
/**
 * Creates an AsyncIterableTransform that takes enveloped messages as a source,
 * and compresses them if they are larger than compressMinBytes.
 *
 * @private Internal code, does not follow semantic versioning.
 */
function transformCompressEnvelope(compression, compressMinBytes) {
    return function (iterable) {
        return __asyncGenerator(this, arguments, function* () {
            var _a, e_8, _b, _c;
            try {
                for (var _d = true, iterable_8 = __asyncValues(iterable), iterable_8_1; iterable_8_1 = yield __await(iterable_8.next()), _a = iterable_8_1.done, !_a; _d = true) {
                    _c = iterable_8_1.value;
                    _d = false;
                    const env = _c;
                    yield yield __await(yield __await(envelopeCompress(env, compression, compressMinBytes)));
                }
            }
            catch (e_8_1) { e_8 = { error: e_8_1 }; }
            finally {
                try {
                    if (!_d && !_a && (_b = iterable_8.return)) yield __await(_b.call(iterable_8));
                }
                finally { if (e_8) throw e_8.error; }
            }
        });
    };
}
/**
 * Creates an AsyncIterableTransform that takes enveloped messages as a source,
 * and decompresses them using the given compression.
 *
 * The iterable raises an error if the decompressed payload of an enveloped
 * message is larger than readMaxBytes, or if no compression is provided.
 *
 * @private Internal code, does not follow semantic versioning.
 */
function transformDecompressEnvelope(compression, readMaxBytes) {
    return function (iterable) {
        return __asyncGenerator(this, arguments, function* () {
            var _a, e_9, _b, _c;
            try {
                for (var _d = true, iterable_9 = __asyncValues(iterable), iterable_9_1; iterable_9_1 = yield __await(iterable_9.next()), _a = iterable_9_1.done, !_a; _d = true) {
                    _c = iterable_9_1.value;
                    _d = false;
                    const env = _c;
                    yield yield __await(yield __await(envelopeDecompress(env, compression, readMaxBytes)));
                }
            }
            catch (e_9_1) { e_9 = { error: e_9_1 }; }
            finally {
                try {
                    if (!_d && !_a && (_b = iterable_9.return)) yield __await(_b.call(iterable_9));
                }
                finally { if (e_9) throw e_9.error; }
            }
        });
    };
}
/**
 * Create an AsyncIterableTransform that takes enveloped messages as a source,
 * and joins them into a stream of raw bytes.
 *
 * @private Internal code, does not follow semantic versioning.
 */
function transformJoinEnvelopes() {
    return function (iterable) {
        return __asyncGenerator(this, arguments, function* () {
            var _a, e_10, _b, _c;
            try {
                for (var _d = true, iterable_10 = __asyncValues(iterable), iterable_10_1; iterable_10_1 = yield __await(iterable_10.next()), _a = iterable_10_1.done, !_a; _d = true) {
                    _c = iterable_10_1.value;
                    _d = false;
                    const { flags, data } = _c;
                    yield yield __await(encodeEnvelope(flags, data));
                }
            }
            catch (e_10_1) { e_10 = { error: e_10_1 }; }
            finally {
                try {
                    if (!_d && !_a && (_b = iterable_10.return)) yield __await(_b.call(iterable_10));
                }
                finally { if (e_10) throw e_10.error; }
            }
        });
    };
}
/**
 * Create an AsyncIterableTransform that takes raw bytes as a source, and splits
 * them into enveloped messages.
 *
 * The iterable raises an error
 * - if the payload of an enveloped message is larger than readMaxBytes,
 * - if the stream ended before an enveloped message fully arrived,
 * - or if the stream ended with extraneous data.
 *
 * @private Internal code, does not follow semantic versioning.
 */
function transformSplitEnvelope(readMaxBytes) {
    return function (iterable) {
        return __asyncGenerator(this, arguments, function* () {
            var _a, e_11, _b, _c;
            const buffer = createEnvelopeDecoder(readMaxBytes);
            try {
                for (var _d = true, iterable_11 = __asyncValues(iterable), iterable_11_1; iterable_11_1 = yield __await(iterable_11.next()), _a = iterable_11_1.done, !_a; _d = true) {
                    _c = iterable_11_1.value;
                    _d = false;
                    const chunk = _c;
                    for (const env of buffer.decode(chunk)) {
                        yield yield __await(env);
                    }
                }
            }
            catch (e_11_1) { e_11 = { error: e_11_1 }; }
            finally {
                try {
                    if (!_d && !_a && (_b = iterable_11.return)) yield __await(_b.call(iterable_11));
                }
                finally { if (e_11) throw e_11.error; }
            }
            if (buffer.byteLength > 0) {
                throw new ConnectError("protocol error: incomplete envelope", Code.InvalidArgument);
            }
        });
    };
}
/**
 * Reads all bytes from the source, and concatenates them to a single Uint8Array.
 *
 * Raises an error if:
 * - more than readMaxBytes are read
 * - lengthHint is a positive integer, but larger than readMaxBytes
 * - lengthHint is a positive integer, and the source contains more or less bytes
 *   than promised
 *
 * @private Internal code, does not follow semantic versioning.
 */
async function readAllBytes(iterable, readMaxBytes, lengthHint) {
    var _a, e_12, _b, _c, _d, e_13, _e, _f;
    const [ok, hint] = parseLengthHint(lengthHint);
    if (ok) {
        if (hint > readMaxBytes) {
            assertReadMaxBytes(readMaxBytes, hint, true);
        }
        const buffer = new Uint8Array(hint);
        let offset = 0;
        try {
            for (var _g = true, iterable_12 = __asyncValues(iterable), iterable_12_1; iterable_12_1 = await iterable_12.next(), _a = iterable_12_1.done, !_a; _g = true) {
                _c = iterable_12_1.value;
                _g = false;
                const chunk = _c;
                if (offset + chunk.byteLength > hint) {
                    throw new ConnectError(`protocol error: promised ${hint} bytes, received ${offset + chunk.byteLength}`, Code.InvalidArgument);
                }
                buffer.set(chunk, offset);
                offset += chunk.byteLength;
            }
        }
        catch (e_12_1) { e_12 = { error: e_12_1 }; }
        finally {
            try {
                if (!_g && !_a && (_b = iterable_12.return)) await _b.call(iterable_12);
            }
            finally { if (e_12) throw e_12.error; }
        }
        if (offset < hint) {
            throw new ConnectError(`protocol error: promised ${hint} bytes, received ${offset}`, Code.InvalidArgument);
        }
        return buffer;
    }
    const chunks = [];
    let count = 0;
    try {
        for (var _h = true, iterable_13 = __asyncValues(iterable), iterable_13_1; iterable_13_1 = await iterable_13.next(), _d = iterable_13_1.done, !_d; _h = true) {
            _f = iterable_13_1.value;
            _h = false;
            const chunk = _f;
            count += chunk.byteLength;
            assertReadMaxBytes(readMaxBytes, count);
            chunks.push(chunk);
        }
    }
    catch (e_13_1) { e_13 = { error: e_13_1 }; }
    finally {
        try {
            if (!_h && !_d && (_e = iterable_13.return)) await _e.call(iterable_13);
        }
        finally { if (e_13) throw e_13.error; }
    }
    const all = new Uint8Array(count);
    let offset = 0;
    for (let chunk = chunks.shift(); chunk; chunk = chunks.shift()) {
        all.set(chunk, offset);
        offset += chunk.byteLength;
    }
    return all;
}
// parse the lengthHint argument of readAllBytes()
function parseLengthHint(lengthHint) {
    if (lengthHint === undefined || lengthHint === null) {
        return [false, 0];
    }
    const n = typeof lengthHint == "string" ? parseInt(lengthHint, 10) : lengthHint;
    if (!Number.isSafeInteger(n) || n < 0) {
        return [false, n];
    }
    return [true, n];
}
/**
 * Wait for the first element of an iterable without modifying the iterable.
 * This consumes the first element, but pushes it back on the stack.
 *
 * @private Internal code, does not follow semantic versioning.
 */
async function untilFirst(iterable) {
    const it = iterable[Symbol.asyncIterator]();
    let first = await it.next();
    return {
        [Symbol.asyncIterator]() {
            const w = {
                async next() {
                    if (first !== null) {
                        const n = first;
                        first = null;
                        return n;
                    }
                    return await it.next();
                },
            };
            if (it.throw !== undefined) {
                w.throw = (e) => it.throw(e);
            }
            if (it.return !== undefined) {
                w.return = (value) => it.return(value);
            }
            return w;
        },
    };
}
/**
 * Wrap the given iterable and return an iterable with an abort() method.
 *
 * This function exists purely for convenience. Where one would typically have
 * to access the iterator directly, advance through all elements, and call
 * AsyncIterator.throw() to notify the upstream iterable, this function allows
 * to use convenient for-await loops and still notify the upstream iterable:
 *
 * ```ts
 * const abortable = makeIterableAbortable(iterable);
 * for await (const ele of abortable) {
 *   await abortable.abort("ERR");
 * }
 * ```
 * There are a couple of limitations of this function:
 * - the given async iterable must implement throw
 * - the async iterable cannot be re-use
 * - if source catches errors and yields values for them, they are ignored, and
 *   the source may still dangle
 *
 * There are four possible ways an async function* can handle yield errors:
 * 1. don't catch errors at all - Abortable.abort() will resolve "rethrown"
 * 2. catch errors and rethrow - Abortable.abort() will resolve "rethrown"
 * 3. catch errors and return - Abortable.abort() will resolve "completed"
 * 4. catch errors and yield a value - Abortable.abort() will resolve "caught"
 *
 * Note that catching errors and yielding a value is problematic, and it should
 * be documented that this may leave the source in a dangling state.
 *
 * @private Internal code, does not follow semantic versioning.
 */
function makeIterableAbortable(iterable) {
    const innerCandidate = iterable[Symbol.asyncIterator]();
    if (innerCandidate.throw === undefined) {
        throw new Error("AsyncIterable does not implement throw");
    }
    const inner = innerCandidate;
    let aborted;
    let resultPromise;
    let it = {
        next() {
            resultPromise = inner.next().finally(() => {
                resultPromise = undefined;
            });
            return resultPromise;
        },
        throw(e) {
            return inner.throw(e);
        },
    };
    if (innerCandidate.return !== undefined) {
        it = Object.assign(Object.assign({}, it), { return(value) {
                return inner.return(value);
            } });
    }
    let used = false;
    return {
        abort(reason) {
            if (aborted) {
                return aborted.state;
            }
            const f = () => {
                return inner.throw(reason).then((r) => (r.done === true ? "completed" : "caught"), () => "rethrown");
            };
            if (resultPromise) {
                aborted = { reason, state: resultPromise.then(f, f) };
                return aborted.state;
            }
            aborted = { reason, state: f() };
            return aborted.state;
        },
        [Symbol.asyncIterator]() {
            if (used) {
                throw new Error("AsyncIterable cannot be re-used");
            }
            used = true;
            return it;
        },
    };
}
/**
 * Create a new WritableIterable.
 */
function createWritableIterable() {
    // We start with two queues to capture the read and write attempts.
    //
    // The writes and reads each check of their counterpart is
    // already available and either interact/add themselves to the queue.
    const readQueue = [];
    const writeQueue = [];
    let err = undefined;
    let nextResolve;
    let nextReject;
    let nextPromise = new Promise((resolve, reject) => {
        nextResolve = resolve;
        nextReject = reject;
    });
    let closed = false;
    // drain the readQueue in case of error/writer is closed by sending a
    // done result.
    function drain() {
        for (const next of readQueue.splice(0, readQueue.length)) {
            next({ done: true, value: undefined });
        }
    }
    return {
        close() {
            closed = true;
            drain();
        },
        async write(payload) {
            if (closed) {
                throw err !== null && err !== void 0 ? err : new Error("cannot write, WritableIterable already closed");
            }
            const read = readQueue.shift();
            if (read === undefined) {
                // We didn't find a pending read so we add the payload to the write queue.
                writeQueue.push(payload);
            }
            else {
                // We found a pending read so we respond with the payload.
                read({ done: false, value: payload });
                if (readQueue.length > 0) {
                    // If there are more in the read queue we can mark the write as complete.
                    // as the error reporting is not guaranteed to be sequential and therefore cannot
                    // to linked to a specific write.
                    return;
                }
            }
            // We await the next call for as many times as there are items in the queue + 1
            //
            // If there are no items in the write queue that means write happened and we just have
            // to wait for one more call likewise if we are the nth write in the queue we
            // have to wait for n writes to complete and one more.
            const limit = writeQueue.length + 1;
            for (let i = 0; i < limit; i++) {
                await nextPromise;
            }
        },
        [Symbol.asyncIterator]() {
            return {
                next() {
                    // Resolve the nextPromise to indicate
                    // pending writes that a read attempt has been made
                    // after their write.
                    //
                    // We also need to reset the promise for future writes.
                    nextResolve();
                    nextPromise = new Promise((resolve, reject) => {
                        nextResolve = resolve;
                        nextReject = reject;
                    });
                    const write = writeQueue.shift();
                    if (write !== undefined) {
                        // We found a pending write so response with the payload.
                        return Promise.resolve({ done: false, value: write });
                    }
                    if (closed) {
                        return Promise.resolve({ done: true, value: undefined });
                    }
                    // We return a promise immediately that is either resolved/rejected
                    // as writes happen.
                    let readResolve;
                    const readPromise = new Promise((resolve) => {
                        readResolve = resolve;
                    });
                    // biome-ignore lint/style/noNonNullAssertion: initialized by promise executor
                    readQueue.push(readResolve);
                    return readPromise;
                },
                throw(throwErr) {
                    err = throwErr;
                    closed = true;
                    writeQueue.splice(0, writeQueue.length);
                    nextPromise.catch(() => {
                        // To make sure that the nextPromise is always resolved.
                    });
                    // This will reject all pending writes.
                    nextReject(err);
                    drain();
                    return Promise.resolve({ done: true, value: undefined });
                },
                return() {
                    closed = true;
                    writeQueue.splice(0, writeQueue.length);
                    // Resolve once for the write awaiting confirmation.
                    nextResolve();
                    // Reject all future writes.
                    nextPromise = Promise.reject(new Error("cannot write, consumer called return"));
                    nextPromise.catch(() => {
                        // To make sure that the nextPromise is always resolved.
                    });
                    drain();
                    return Promise.resolve({ done: true, value: undefined });
                },
            };
        },
    };
}
/**
 * Create an asynchronous iterable from an array.
 *
 * @private Internal code, does not follow semantic versioning.
 */
function createAsyncIterable(items) {
    return __asyncGenerator(this, arguments, function* createAsyncIterable_1() {
        yield __await(yield* __asyncDelegator(__asyncValues(items)));
    });
}

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/promise-client.js
// Copyright 2021-2025 The Connect Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
var promise_client_asyncValues = (undefined && undefined.__asyncValues) || function (o) {
    if (!Symbol.asyncIterator) throw new TypeError("Symbol.asyncIterator is not defined.");
    var m = o[Symbol.asyncIterator], i;
    return m ? m.call(o) : (o = typeof __values === "function" ? __values(o) : o[Symbol.iterator](), i = {}, verb("next"), verb("throw"), verb("return"), i[Symbol.asyncIterator] = function () { return this; }, i);
    function verb(n) { i[n] = o[n] && function (v) { return new Promise(function (resolve, reject) { v = o[n](v), settle(resolve, reject, v.done, v.value); }); }; }
    function settle(resolve, reject, d, v) { Promise.resolve(v).then(function(v) { resolve({ value: v, done: d }); }, reject); }
};
var promise_client_await = (undefined && undefined.__await) || function (v) { return this instanceof promise_client_await ? (this.v = v, this) : new promise_client_await(v); }
var promise_client_asyncDelegator = (undefined && undefined.__asyncDelegator) || function (o) {
    var i, p;
    return i = {}, verb("next"), verb("throw", function (e) { throw e; }), verb("return"), i[Symbol.iterator] = function () { return this; }, i;
    function verb(n, f) { i[n] = o[n] ? function (v) { return (p = !p) ? { value: promise_client_await(o[n](v)), done: false } : f ? f(v) : v; } : f; }
};
var promise_client_asyncGenerator = (undefined && undefined.__asyncGenerator) || function (thisArg, _arguments, generator) {
    if (!Symbol.asyncIterator) throw new TypeError("Symbol.asyncIterator is not defined.");
    var g = generator.apply(thisArg, _arguments || []), i, q = [];
    return i = Object.create((typeof AsyncIterator === "function" ? AsyncIterator : Object).prototype), verb("next"), verb("throw"), verb("return", awaitReturn), i[Symbol.asyncIterator] = function () { return this; }, i;
    function awaitReturn(f) { return function (v) { return Promise.resolve(v).then(f, reject); }; }
    function verb(n, f) { if (g[n]) { i[n] = function (v) { return new Promise(function (a, b) { q.push([n, v, a, b]) > 1 || resume(n, v); }); }; if (f) i[n] = f(i[n]); } }
    function resume(n, v) { try { step(g[n](v)); } catch (e) { settle(q[0][3], e); } }
    function step(r) { r.value instanceof promise_client_await ? Promise.resolve(r.value.v).then(fulfill, reject) : settle(q[0][2], r); }
    function fulfill(value) { resume("next", value); }
    function reject(value) { resume("throw", value); }
    function settle(f, v) { if (f(v), q.shift(), q.length) resume(q[0][0], q[0][1]); }
};




/**
 * Create a Client for the given service, invoking RPCs through the
 * given transport.
 */
function createClient(service, transport) {
    return makeAnyClient(service, (method) => {
        switch (method.methodKind) {
            case "unary":
                return createUnaryFn(transport, method);
            case "server_streaming":
                return createServerStreamingFn(transport, method);
            case "client_streaming":
                return createClientStreamingFn(transport, method);
            case "bidi_streaming":
                return createBiDiStreamingFn(transport, method);
            default:
                return null;
        }
    });
}
function createUnaryFn(transport, method) {
    return async (input, options) => {
        var _a, _b;
        const response = await transport.unary(method, options === null || options === void 0 ? void 0 : options.signal, options === null || options === void 0 ? void 0 : options.timeoutMs, options === null || options === void 0 ? void 0 : options.headers, input, options === null || options === void 0 ? void 0 : options.contextValues);
        (_a = options === null || options === void 0 ? void 0 : options.onHeader) === null || _a === void 0 ? void 0 : _a.call(options, response.header);
        (_b = options === null || options === void 0 ? void 0 : options.onTrailer) === null || _b === void 0 ? void 0 : _b.call(options, response.trailer);
        return response.message;
    };
}
function createServerStreamingFn(transport, method) {
    return (input, options) => handleStreamResponse(transport.stream(method, options === null || options === void 0 ? void 0 : options.signal, options === null || options === void 0 ? void 0 : options.timeoutMs, options === null || options === void 0 ? void 0 : options.headers, createAsyncIterable([input]), options === null || options === void 0 ? void 0 : options.contextValues), options);
}
function createClientStreamingFn(transport, method) {
    return async (request, options) => {
        var _a, e_1, _b, _c;
        var _d, _e;
        const response = await transport.stream(method, options === null || options === void 0 ? void 0 : options.signal, options === null || options === void 0 ? void 0 : options.timeoutMs, options === null || options === void 0 ? void 0 : options.headers, request, options === null || options === void 0 ? void 0 : options.contextValues);
        (_d = options === null || options === void 0 ? void 0 : options.onHeader) === null || _d === void 0 ? void 0 : _d.call(options, response.header);
        let singleMessage;
        let count = 0;
        try {
            for (var _f = true, _g = promise_client_asyncValues(response.message), _h; _h = await _g.next(), _a = _h.done, !_a; _f = true) {
                _c = _h.value;
                _f = false;
                const message = _c;
                singleMessage = message;
                count++;
            }
        }
        catch (e_1_1) { e_1 = { error: e_1_1 }; }
        finally {
            try {
                if (!_f && !_a && (_b = _g.return)) await _b.call(_g);
            }
            finally { if (e_1) throw e_1.error; }
        }
        if (!singleMessage) {
            throw new connect_error/* ConnectError */.T("protocol error: missing response message", code/* Code */.C.Unimplemented);
        }
        if (count > 1) {
            throw new connect_error/* ConnectError */.T("protocol error: received extra messages for client streaming method", code/* Code */.C.Unimplemented);
        }
        (_e = options === null || options === void 0 ? void 0 : options.onTrailer) === null || _e === void 0 ? void 0 : _e.call(options, response.trailer);
        return singleMessage;
    };
}
function createBiDiStreamingFn(transport, method) {
    return (request, options) => handleStreamResponse(transport.stream(method, options === null || options === void 0 ? void 0 : options.signal, options === null || options === void 0 ? void 0 : options.timeoutMs, options === null || options === void 0 ? void 0 : options.headers, request, options === null || options === void 0 ? void 0 : options.contextValues), options);
}
function handleStreamResponse(stream, options) {
    const it = (function () {
        return promise_client_asyncGenerator(this, arguments, function* () {
            var _a, _b;
            const response = yield promise_client_await(stream);
            (_a = options === null || options === void 0 ? void 0 : options.onHeader) === null || _a === void 0 ? void 0 : _a.call(options, response.header);
            yield promise_client_await(yield* promise_client_asyncDelegator(promise_client_asyncValues(response.message)));
            (_b = options === null || options === void 0 ? void 0 : options.onTrailer) === null || _b === void 0 ? void 0 : _b.call(options, response.trailer);
        });
    })()[Symbol.asyncIterator]();
    // Create a new iterable to omit throw/return.
    return {
        [Symbol.asyncIterator]: () => ({
            next: () => it.next(),
        }),
    };
}


/***/ }),

/***/ 69522:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   j: () => (/* binding */ createMethodUrl)
/* harmony export */ });
// Copyright 2021-2025 The Connect Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
/**
 * Create a URL for the given RPC. This simply adds the qualified
 * service name, a slash, and the method name to the path of the given
 * baseUrl.
 *
 * For example, the baseUri https://example.com and method "Say" from
 * the service example.ElizaService results in:
 * https://example.com/example.ElizaService/Say
 *
 * This format is used by the protocols Connect, gRPC and Twirp.
 *
 * Note that this function also accepts a protocol-relative baseUrl.
 * If given an empty string or "/" as a baseUrl, it returns just the
 * path.
 */
function createMethodUrl(baseUrl, method) {
    return baseUrl
        .toString()
        .replace(/\/?$/, `/${method.parent.typeName}/${method.name}`);
}


/***/ }),

/***/ 70050:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  Ef: () => (/* binding */ createClientMethodSerializers),
  B_: () => (/* binding */ getJsonOptions)
});

// UNUSED EXPORTS: createBinarySerialization, createJsonSerialization, createMethodSerializationLookup, limitSerialization

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/from-binary.js
var from_binary = __webpack_require__(57386);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/to-binary.js
var to_binary = __webpack_require__(2681);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/from-json.js + 1 modules
var from_json = __webpack_require__(89748);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/descriptors.js
var descriptors = __webpack_require__(92806);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/reflect/names.js
var names = __webpack_require__(35250);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/reflect/reflect.js
var reflect = __webpack_require__(64683);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/wkt/any.js + 1 modules
var any = __webpack_require__(10706);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/wkt/wrappers.js
var wrappers = __webpack_require__(34123);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/wire/base64-encoding.js
var base64_encoding = __webpack_require__(14097);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/extensions.js
var extensions = __webpack_require__(65402);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/reflect/reflect-check.js
var reflect_check = __webpack_require__(45394);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/to-json.js
// Copyright 2021-2026 Buf Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.








// bootstrap-inject google.protobuf.FeatureSet.FieldPresence.LEGACY_REQUIRED: const $name: FeatureSet_FieldPresence.$localName = $number;
const LEGACY_REQUIRED = 3;
// bootstrap-inject google.protobuf.FeatureSet.FieldPresence.IMPLICIT: const $name: FeatureSet_FieldPresence.$localName = $number;
const IMPLICIT = 2;
// Default options for serializing to JSON.
const jsonWriteDefaults = {
    alwaysEmitImplicit: false,
    enumAsInteger: false,
    useProtoFieldName: false,
};
function makeWriteOptions(options) {
    return options ? Object.assign(Object.assign({}, jsonWriteDefaults), options) : jsonWriteDefaults;
}
/**
 * Serialize the message to a JSON value, a JavaScript value that can be
 * passed to JSON.stringify().
 */
function toJson(schema, message, options) {
    return reflectToJson((0,reflect/* reflect */.E4)(schema, message), makeWriteOptions(options));
}
/**
 * Serialize the message to a JSON string.
 */
function toJsonString(schema, message, options) {
    var _a;
    const jsonValue = toJson(schema, message, options);
    return JSON.stringify(jsonValue, null, (_a = options === null || options === void 0 ? void 0 : options.prettySpaces) !== null && _a !== void 0 ? _a : 0);
}
/**
 * Serialize a single enum value to JSON.
 */
function enumToJson(descEnum, value) {
    var _a;
    if (descEnum.typeName == "google.protobuf.NullValue") {
        return null;
    }
    const name = (_a = descEnum.value[value]) === null || _a === void 0 ? void 0 : _a.name;
    if (name === undefined) {
        throw new Error(`${value} is not a value in ${descEnum}`);
    }
    return name;
}
function reflectToJson(msg, opts) {
    var _a;
    const wktJson = tryWktToJson(msg, opts);
    if (wktJson !== undefined)
        return wktJson;
    const json = {};
    for (const f of msg.sortedFields) {
        if (!msg.isSet(f)) {
            if (f.presence == LEGACY_REQUIRED) {
                throw new Error(`cannot encode ${f} to JSON: required field not set`);
            }
            if (!opts.alwaysEmitImplicit || f.presence !== IMPLICIT) {
                // Fields with implicit presence omit zero values (e.g. empty string) by default
                continue;
            }
        }
        const jsonValue = fieldToJson(f, msg.get(f), opts);
        if (jsonValue !== undefined) {
            json[jsonName(f, opts)] = jsonValue;
        }
    }
    if (opts.registry) {
        const tagSeen = new Set();
        for (const { no } of (_a = msg.getUnknown()) !== null && _a !== void 0 ? _a : []) {
            // Same tag can appear multiple times, so we
            // keep track and skip identical ones.
            if (!tagSeen.has(no)) {
                tagSeen.add(no);
                const extension = opts.registry.getExtensionFor(msg.desc, no);
                if (!extension) {
                    continue;
                }
                const value = (0,extensions/* getExtension */.uQ)(msg.message, extension);
                const [container, field] = (0,extensions/* createExtensionContainer */.z3)(extension, value);
                const jsonValue = fieldToJson(field, container.get(field), opts);
                if (jsonValue !== undefined) {
                    json[extension.jsonName] = jsonValue;
                }
            }
        }
    }
    return json;
}
function fieldToJson(f, val, opts) {
    switch (f.fieldKind) {
        case "scalar":
            return scalarToJson(f, val);
        case "message":
            return reflectToJson(val, opts);
        case "enum":
            return enumToJsonInternal(f.enum, val, opts.enumAsInteger);
        case "list":
            return listToJson(val, opts);
        case "map":
            return mapToJson(val, opts);
    }
}
function mapToJson(map, opts) {
    const f = map.field();
    const jsonObj = {};
    switch (f.mapKind) {
        case "scalar":
            for (const [entryKey, entryValue] of map) {
                jsonObj[entryKey] = scalarToJson(f, entryValue);
            }
            break;
        case "message":
            for (const [entryKey, entryValue] of map) {
                jsonObj[entryKey] = reflectToJson(entryValue, opts);
            }
            break;
        case "enum":
            for (const [entryKey, entryValue] of map) {
                jsonObj[entryKey] = enumToJsonInternal(f.enum, entryValue, opts.enumAsInteger);
            }
            break;
    }
    return opts.alwaysEmitImplicit || map.size > 0 ? jsonObj : undefined;
}
function listToJson(list, opts) {
    const f = list.field();
    const jsonArr = [];
    switch (f.listKind) {
        case "scalar":
            for (const item of list) {
                jsonArr.push(scalarToJson(f, item));
            }
            break;
        case "enum":
            for (const item of list) {
                jsonArr.push(enumToJsonInternal(f.enum, item, opts.enumAsInteger));
            }
            break;
        case "message":
            for (const item of list) {
                jsonArr.push(reflectToJson(item, opts));
            }
            break;
    }
    return opts.alwaysEmitImplicit || jsonArr.length > 0 ? jsonArr : undefined;
}
function enumToJsonInternal(desc, value, enumAsInteger) {
    var _a;
    if (typeof value != "number") {
        throw new Error(`cannot encode ${desc} to JSON: expected number, got ${(0,reflect_check/* formatVal */.$L)(value)}`);
    }
    if (desc.typeName == "google.protobuf.NullValue") {
        return null;
    }
    if (enumAsInteger) {
        return value;
    }
    const val = desc.value[value];
    return (_a = val === null || val === void 0 ? void 0 : val.name) !== null && _a !== void 0 ? _a : value; // if we don't know the enum value, just return the number
}
function scalarToJson(field, value) {
    var _a, _b, _c, _d, _e, _f;
    switch (field.scalar) {
        // int32, fixed32, uint32: JSON value will be a decimal number. Either numbers or strings are accepted.
        case descriptors/* ScalarType */.L.INT32:
        case descriptors/* ScalarType */.L.SFIXED32:
        case descriptors/* ScalarType */.L.SINT32:
        case descriptors/* ScalarType */.L.FIXED32:
        case descriptors/* ScalarType */.L.UINT32:
            if (typeof value != "number") {
                throw new Error(`cannot encode ${field} to JSON: ${(_a = (0,reflect_check/* checkField */.PZ)(field, value)) === null || _a === void 0 ? void 0 : _a.message}`);
            }
            return value;
        // float, double: JSON value will be a number or one of the special string values "NaN", "Infinity", and "-Infinity".
        // Either numbers or strings are accepted. Exponent notation is also accepted.
        case descriptors/* ScalarType */.L.FLOAT:
        case descriptors/* ScalarType */.L.DOUBLE: // eslint-disable-line no-fallthrough
            if (typeof value != "number") {
                throw new Error(`cannot encode ${field} to JSON: ${(_b = (0,reflect_check/* checkField */.PZ)(field, value)) === null || _b === void 0 ? void 0 : _b.message}`);
            }
            if (Number.isNaN(value))
                return "NaN";
            if (value === Number.POSITIVE_INFINITY)
                return "Infinity";
            if (value === Number.NEGATIVE_INFINITY)
                return "-Infinity";
            return value;
        // string:
        case descriptors/* ScalarType */.L.STRING:
            if (typeof value != "string") {
                throw new Error(`cannot encode ${field} to JSON: ${(_c = (0,reflect_check/* checkField */.PZ)(field, value)) === null || _c === void 0 ? void 0 : _c.message}`);
            }
            return value;
        // bool:
        case descriptors/* ScalarType */.L.BOOL:
            if (typeof value != "boolean") {
                throw new Error(`cannot encode ${field} to JSON: ${(_d = (0,reflect_check/* checkField */.PZ)(field, value)) === null || _d === void 0 ? void 0 : _d.message}`);
            }
            return value;
        // JSON value will be a decimal string. Either numbers or strings are accepted.
        case descriptors/* ScalarType */.L.UINT64:
        case descriptors/* ScalarType */.L.FIXED64:
        case descriptors/* ScalarType */.L.INT64:
        case descriptors/* ScalarType */.L.SFIXED64:
        case descriptors/* ScalarType */.L.SINT64:
            if (typeof value != "bigint" && typeof value != "string") {
                throw new Error(`cannot encode ${field} to JSON: ${(_e = (0,reflect_check/* checkField */.PZ)(field, value)) === null || _e === void 0 ? void 0 : _e.message}`);
            }
            return value.toString();
        // bytes: JSON value will be the data encoded as a string using standard base64 encoding with paddings.
        // Either standard or URL-safe base64 encoding with/without paddings are accepted.
        case descriptors/* ScalarType */.L.BYTES:
            if (value instanceof Uint8Array) {
                return (0,base64_encoding/* base64Encode */.K)(value);
            }
            throw new Error(`cannot encode ${field} to JSON: ${(_f = (0,reflect_check/* checkField */.PZ)(field, value)) === null || _f === void 0 ? void 0 : _f.message}`);
    }
}
function jsonName(f, opts) {
    return opts.useProtoFieldName ? f.name : f.jsonName;
}
// returns a json value if wkt, otherwise returns undefined.
function tryWktToJson(msg, opts) {
    if (!msg.desc.typeName.startsWith("google.protobuf.")) {
        return undefined;
    }
    switch (msg.desc.typeName) {
        case "google.protobuf.Any":
            return anyToJson(msg.message, opts);
        case "google.protobuf.Timestamp":
            return timestampToJson(msg.message);
        case "google.protobuf.Duration":
            return durationToJson(msg.message);
        case "google.protobuf.FieldMask":
            return fieldMaskToJson(msg.message);
        case "google.protobuf.Struct":
            return structToJson(msg.message);
        case "google.protobuf.Value":
            return valueToJson(msg.message);
        case "google.protobuf.ListValue":
            return listValueToJson(msg.message);
        default:
            if ((0,wrappers/* isWrapperDesc */.l)(msg.desc)) {
                const valueField = msg.desc.fields[0];
                return scalarToJson(valueField, msg.get(valueField));
            }
            return undefined;
    }
}
function anyToJson(val, opts) {
    if (val.typeUrl === "") {
        return {};
    }
    const { registry } = opts;
    let message;
    let desc;
    if (registry) {
        message = (0,any/* anyUnpack */.rW)(val, registry);
        if (message) {
            desc = registry.getMessage(message.$typeName);
        }
    }
    if (!desc || !message) {
        throw new Error(`cannot encode message ${val.$typeName} to JSON: "${val.typeUrl}" is not in the type registry`);
    }
    let json = reflectToJson((0,reflect/* reflect */.E4)(desc, message), opts);
    if (desc.typeName.startsWith("google.protobuf.") ||
        json === null ||
        Array.isArray(json) ||
        typeof json !== "object") {
        json = { value: json };
    }
    json["@type"] = val.typeUrl;
    return json;
}
function durationToJson(val) {
    const seconds = Number(val.seconds);
    const nanos = val.nanos;
    if (seconds > 315576000000 || seconds < -315576000000) {
        throw new Error(`cannot encode message ${val.$typeName} to JSON: value out of range`);
    }
    if ((seconds > 0 && nanos < 0) || (seconds < 0 && nanos > 0)) {
        throw new Error(`cannot encode message ${val.$typeName} to JSON: nanos sign must match seconds sign`);
    }
    let text = val.seconds.toString();
    if (nanos !== 0) {
        let nanosStr = Math.abs(nanos).toString();
        nanosStr = "0".repeat(9 - nanosStr.length) + nanosStr;
        if (nanosStr.substring(3) === "000000") {
            nanosStr = nanosStr.substring(0, 3);
        }
        else if (nanosStr.substring(6) === "000") {
            nanosStr = nanosStr.substring(0, 6);
        }
        text += "." + nanosStr;
        if (nanos < 0 && seconds == 0) {
            text = "-" + text;
        }
    }
    return text + "s";
}
function fieldMaskToJson(val) {
    return val.paths
        .map((p) => {
        if ((0,names/* protoSnakeCase */.t5)((0,names/* protoCamelCase */.RM)(p)) !== p) {
            throw new Error(`cannot encode message ${val.$typeName} to JSON: lowerCamelCase of path name "${p}" is irreversible`);
        }
        return (0,names/* protoCamelCase */.RM)(p);
    })
        .join(",");
}
function structToJson(val) {
    const json = {};
    for (const [k, v] of Object.entries(val.fields)) {
        json[k] = valueToJson(v);
    }
    return json;
}
function valueToJson(val) {
    switch (val.kind.case) {
        case "nullValue":
            return null;
        case "numberValue":
            if (!Number.isFinite(val.kind.value)) {
                throw new Error(`${val.$typeName} cannot be NaN or Infinity`);
            }
            return val.kind.value;
        case "boolValue":
            return val.kind.value;
        case "stringValue":
            return val.kind.value;
        case "structValue":
            return structToJson(val.kind.value);
        case "listValue":
            return listValueToJson(val.kind.value);
        default:
            throw new Error(`${val.$typeName} must have a value`);
    }
}
function listValueToJson(val) {
    return val.values.map(valueToJson);
}
function timestampToJson(val) {
    const ms = Number(val.seconds) * 1000;
    if (ms < Date.parse("0001-01-01T00:00:00Z") ||
        ms > Date.parse("9999-12-31T23:59:59Z")) {
        throw new Error(`cannot encode message ${val.$typeName} to JSON: must be from 0001-01-01T00:00:00Z to 9999-12-31T23:59:59Z inclusive`);
    }
    if (val.nanos < 0) {
        throw new Error(`cannot encode message ${val.$typeName} to JSON: nanos must not be negative`);
    }
    if (val.nanos > 999999999) {
        throw new Error(`cannot encode message ${val.$typeName} to JSON: nanos must not be greater than 99999999`);
    }
    let z = "Z";
    if (val.nanos > 0) {
        const nanosStr = (val.nanos + 1000000000).toString().substring(1);
        if (nanosStr.substring(3) === "000000") {
            z = "." + nanosStr.substring(0, 3) + "Z";
        }
        else if (nanosStr.substring(6) === "000") {
            z = "." + nanosStr.substring(0, 6) + "Z";
        }
        else {
            z = "." + nanosStr + "Z";
        }
    }
    return new Date(ms).toISOString().replace(".000Z", z);
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/connect-error.js
var connect_error = __webpack_require__(45640);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/code.js
var code = __webpack_require__(16294);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/protocol/serialization.js
// Copyright 2021-2025 The Connect Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.




/**
 * Sets default JSON serialization options for connect-es.
 *
 * With standard protobuf JSON serialization, unknown JSON fields are
 * rejected by default. In connect-es, unknown JSON fields are ignored
 * by default.
 */
function getJsonOptions(options) {
    var _a;
    const o = Object.assign({}, options);
    (_a = o.ignoreUnknownFields) !== null && _a !== void 0 ? _a : (o.ignoreUnknownFields = true);
    return o;
}
/**
 * Create an object that provides convenient access to request and response
 * message serialization for a given method.
 *
 * @private Internal code, does not follow semantic versioning.
 */
function createMethodSerializationLookup(method, binaryOptions, jsonOptions, limitOptions) {
    const inputBinary = limitSerialization(createBinarySerialization(method.input, binaryOptions), limitOptions);
    const inputJson = limitSerialization(createJsonSerialization(method.input, jsonOptions), limitOptions);
    const outputBinary = limitSerialization(createBinarySerialization(method.output, binaryOptions), limitOptions);
    const outputJson = limitSerialization(createJsonSerialization(method.output, jsonOptions), limitOptions);
    return {
        getI(useBinaryFormat) {
            return useBinaryFormat ? inputBinary : inputJson;
        },
        getO(useBinaryFormat) {
            return useBinaryFormat ? outputBinary : outputJson;
        },
    };
}
/**
 * Returns functions to normalize and serialize the input message
 * of an RPC, and to parse the output message of an RPC.
 *
 * @private Internal code, does not follow semantic versioning.
 */
function createClientMethodSerializers(method, useBinaryFormat, jsonOptions, binaryOptions) {
    const input = useBinaryFormat
        ? createBinarySerialization(method.input, binaryOptions)
        : createJsonSerialization(method.input, jsonOptions);
    const output = useBinaryFormat
        ? createBinarySerialization(method.output, binaryOptions)
        : createJsonSerialization(method.output, jsonOptions);
    return { parse: output.parse, serialize: input.serialize };
}
/**
 * Apply I/O limits to a Serialization object, returning a new object.
 *
 * @private Internal code, does not follow semantic versioning.
 */
function limitSerialization(serialization, limitOptions) {
    return {
        serialize(data) {
            const bytes = serialization.serialize(data);
            assertWriteMaxBytes(limitOptions.writeMaxBytes, bytes.byteLength);
            return bytes;
        },
        parse(data) {
            assertReadMaxBytes(limitOptions.readMaxBytes, data.byteLength, true);
            return serialization.parse(data);
        },
    };
}
/**
 * Creates a Serialization object for serializing the given protobuf message
 * with the protobuf binary format.
 */
function createBinarySerialization(desc, options) {
    return {
        parse(data) {
            try {
                return (0,from_binary/* fromBinary */.Er)(desc, data, options);
            }
            catch (e) {
                const m = e instanceof Error ? e.message : String(e);
                throw new connect_error/* ConnectError */.T(`parse binary: ${m}`, code/* Code */.C.Internal);
            }
        },
        serialize(data) {
            try {
                return (0,to_binary/* toBinary */.R)(desc, data, options);
            }
            catch (e) {
                const m = e instanceof Error ? e.message : String(e);
                throw new connect_error/* ConnectError */.T(`serialize binary: ${m}`, code/* Code */.C.Internal);
            }
        },
    };
}
/**
 * Creates a Serialization object for serializing the given protobuf message
 * with the protobuf canonical JSON encoding.
 *
 * By default, unknown fields are ignored.
 */
function createJsonSerialization(desc, options) {
    var _a, _b;
    const textEncoder = (_a = options === null || options === void 0 ? void 0 : options.textEncoder) !== null && _a !== void 0 ? _a : new TextEncoder();
    const textDecoder = (_b = options === null || options === void 0 ? void 0 : options.textDecoder) !== null && _b !== void 0 ? _b : new TextDecoder();
    const o = getJsonOptions(options);
    return {
        parse(data) {
            try {
                const json = textDecoder.decode(data);
                return (0,from_json/* fromJsonString */.mj)(desc, json, o);
            }
            catch (e) {
                throw connect_error/* ConnectError */.T.from(e, code/* Code */.C.InvalidArgument);
            }
        },
        serialize(data) {
            try {
                const json = toJsonString(desc, data, o);
                return textEncoder.encode(json);
            }
            catch (e) {
                throw connect_error/* ConnectError */.T.from(e, code/* Code */.C.Internal);
            }
        },
    };
}


/***/ }),

/***/ 70525:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Br: () => (/* binding */ contentTypeUnaryJson),
/* harmony export */   Sg: () => (/* binding */ contentTypeStreamProto),
/* harmony export */   Vs: () => (/* binding */ contentTypeUnaryProto),
/* harmony export */   xk: () => (/* binding */ parseContentType),
/* harmony export */   yU: () => (/* binding */ contentTypeStreamJson)
/* harmony export */ });
/* unused harmony exports contentTypeRegExp, contentTypeUnaryRegExp, contentTypeStreamRegExp, parseEncodingQuery */
// Copyright 2021-2025 The Connect Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
/**
 * Regular Expression that matches any valid Connect Content-Type header value.
 *
 * @private Internal code, does not follow semantic versioning.
 */
const contentTypeRegExp = /^application\/(connect\+)?(?:(json)(?:; ?charset=utf-?8)?|(proto))$/i;
/**
 * Regular Expression that matches a Connect unary Content-Type header value.
 *
 * @private Internal code, does not follow semantic versioning.
 */
const contentTypeUnaryRegExp = /^application\/(?:json(?:; ?charset=utf-?8)?|proto)$/i;
/**
 * Regular Expression that matches a Connect streaming Content-Type header value.
 *
 * @private Internal code, does not follow semantic versioning.
 */
const contentTypeStreamRegExp = /^application\/connect\+?(?:json(?:; ?charset=utf-?8)?|proto)$/i;
const contentTypeUnaryProto = "application/proto";
const contentTypeUnaryJson = "application/json";
const contentTypeStreamProto = "application/connect+proto";
const contentTypeStreamJson = "application/connect+json";
const encodingProto = "proto";
const encodingJson = "json";
/**
 * Parse a Connect Content-Type header.
 *
 * @private Internal code, does not follow semantic versioning.
 */
function parseContentType(contentType) {
    const match = contentType === null || contentType === void 0 ? void 0 : contentType.match(contentTypeRegExp);
    if (!match) {
        return undefined;
    }
    const stream = !!match[1];
    const binary = !!match[3];
    return { stream, binary };
}
/**
 * Parse a Connect Get encoding query parameter.
 *
 * @private Internal code, does not follow semantic versioning.
 */
function parseEncodingQuery(encoding) {
    switch (encoding) {
        case encodingProto:
            return { stream: false, binary: true };
        case encodingJson:
            return { stream: false, binary: false };
        default:
            return undefined;
    }
}


/***/ }),

/***/ 72101:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Eu: () => (/* binding */ headerUnaryAcceptEncoding),
/* harmony export */   Z_: () => (/* binding */ headerContentType),
/* harmony export */   _U: () => (/* binding */ headerProtocolVersion),
/* harmony export */   hR: () => (/* binding */ headerTimeout),
/* harmony export */   kq: () => (/* binding */ headerUnaryEncoding),
/* harmony export */   qs: () => (/* binding */ headerUnaryContentLength),
/* harmony export */   qy: () => (/* binding */ headerUserAgent)
/* harmony export */ });
/* unused harmony exports headerStreamEncoding, headerStreamAcceptEncoding */
// Copyright 2021-2025 The Connect Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
/**
 * @private Internal code, does not follow semantic versioning.
 */
const headerContentType = "Content-Type";
const headerUnaryContentLength = "Content-Length";
const headerUnaryEncoding = "Content-Encoding";
const headerStreamEncoding = "Connect-Content-Encoding";
const headerUnaryAcceptEncoding = "Accept-Encoding";
const headerStreamAcceptEncoding = "Connect-Accept-Encoding";
const headerTimeout = "Connect-Timeout-Ms";
const headerProtocolVersion = "Connect-Protocol-Version";
const headerUserAgent = "User-Agent";


/***/ }),

/***/ 89748:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  Rf: () => (/* binding */ fromJson),
  mj: () => (/* binding */ fromJsonString)
});

// UNUSED EXPORTS: enumFromJson, isEnumJson, mergeFromJson, mergeFromJsonString

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/descriptors.js
var descriptors = __webpack_require__(92806);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/proto-int64.js
var proto_int64 = __webpack_require__(2952);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/create.js
var create = __webpack_require__(95444);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/reflect/reflect.js
var reflect_reflect = __webpack_require__(64683);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/reflect/error.js
var error = __webpack_require__(64444);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/reflect/reflect-check.js
var reflect_check = __webpack_require__(45394);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/reflect/names.js
var names = __webpack_require__(35250);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/wire/base64-encoding.js
var base64_encoding = __webpack_require__(14097);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/wkt/wrappers.js
var wrappers = __webpack_require__(34123);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/wkt/any.js + 1 modules
var wkt_any = __webpack_require__(10706);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/codegenv2/file.js
var file = __webpack_require__(1844);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/codegenv2/message.js
var message = __webpack_require__(15033);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/wkt/gen/google/protobuf/struct_pb.js
// Copyright 2021-2026 Buf Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.



/**
 * Describes the file google/protobuf/struct.proto.
 */
const file_google_protobuf_struct = /*@__PURE__*/ (0,file/* fileDesc */.w)("Chxnb29nbGUvcHJvdG9idWYvc3RydWN0LnByb3RvEg9nb29nbGUucHJvdG9idWYihAEKBlN0cnVjdBIzCgZmaWVsZHMYASADKAsyIy5nb29nbGUucHJvdG9idWYuU3RydWN0LkZpZWxkc0VudHJ5GkUKC0ZpZWxkc0VudHJ5EgsKA2tleRgBIAEoCRIlCgV2YWx1ZRgCIAEoCzIWLmdvb2dsZS5wcm90b2J1Zi5WYWx1ZToCOAEi6gEKBVZhbHVlEjAKCm51bGxfdmFsdWUYASABKA4yGi5nb29nbGUucHJvdG9idWYuTnVsbFZhbHVlSAASFgoMbnVtYmVyX3ZhbHVlGAIgASgBSAASFgoMc3RyaW5nX3ZhbHVlGAMgASgJSAASFAoKYm9vbF92YWx1ZRgEIAEoCEgAEi8KDHN0cnVjdF92YWx1ZRgFIAEoCzIXLmdvb2dsZS5wcm90b2J1Zi5TdHJ1Y3RIABIwCgpsaXN0X3ZhbHVlGAYgASgLMhouZ29vZ2xlLnByb3RvYnVmLkxpc3RWYWx1ZUgAQgYKBGtpbmQiMwoJTGlzdFZhbHVlEiYKBnZhbHVlcxgBIAMoCzIWLmdvb2dsZS5wcm90b2J1Zi5WYWx1ZSobCglOdWxsVmFsdWUSDgoKTlVMTF9WQUxVRRAAQn8KE2NvbS5nb29nbGUucHJvdG9idWZCC1N0cnVjdFByb3RvUAFaL2dvb2dsZS5nb2xhbmcub3JnL3Byb3RvYnVmL3R5cGVzL2tub3duL3N0cnVjdHBi+AEBogIDR1BCqgIeR29vZ2xlLlByb3RvYnVmLldlbGxLbm93blR5cGVzYgZwcm90bzM");
/**
 * Describes the message google.protobuf.Struct.
 * Use `create(StructSchema)` to create a new message.
 */
const StructSchema = /*@__PURE__*/ (0,message/* messageDesc */.Z)(file_google_protobuf_struct, 0);
/**
 * Describes the message google.protobuf.Value.
 * Use `create(ValueSchema)` to create a new message.
 */
const ValueSchema = /*@__PURE__*/ (0,message/* messageDesc */.Z)(file_google_protobuf_struct, 1);
/**
 * Describes the message google.protobuf.ListValue.
 * Use `create(ListValueSchema)` to create a new message.
 */
const ListValueSchema = /*@__PURE__*/ (0,message/* messageDesc */.Z)(file_google_protobuf_struct, 2);
/**
 * `NullValue` is a singleton enumeration to represent the null value for the
 * `Value` type union.
 *
 * The JSON representation for `NullValue` is JSON `null`.
 *
 * @generated from enum google.protobuf.NullValue
 */
var NullValue;
(function (NullValue) {
    /**
     * Null value.
     *
     * @generated from enum value: NULL_VALUE = 0;
     */
    NullValue[NullValue["NULL_VALUE"] = 0] = "NULL_VALUE";
})(NullValue || (NullValue = {}));
/**
 * Describes the enum google.protobuf.NullValue.
 */
const NullValueSchema = /*@__PURE__*/ (/* unused pure expression or super */ null && (enumDesc(file_google_protobuf_struct, 0)));

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/extensions.js
var extensions = __webpack_require__(65402);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/from-json.js
// Copyright 2021-2026 Buf Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.










// Default options for parsing JSON.
const jsonReadDefaults = {
    ignoreUnknownFields: false,
};
function makeReadOptions(options) {
    return options ? Object.assign(Object.assign({}, jsonReadDefaults), options) : jsonReadDefaults;
}
/**
 * Parse a message from a JSON string.
 */
function fromJsonString(schema, json, options) {
    return fromJson(schema, parseJsonString(json, schema.typeName), options);
}
/**
 * Parse a message from a JSON string, merging fields.
 *
 * Repeated fields are appended. Map entries are added, overwriting
 * existing keys.
 *
 * If a message field is already present, it will be merged with the
 * new data.
 */
function mergeFromJsonString(schema, target, json, options) {
    return mergeFromJson(schema, target, parseJsonString(json, schema.typeName), options);
}
/**
 * Parse a message from a JSON value.
 */
function fromJson(schema, json, options) {
    const msg = (0,reflect_reflect/* reflect */.E4)(schema);
    try {
        readMessage(msg, json, makeReadOptions(options));
    }
    catch (e) {
        if ((0,error/* isFieldError */.p)(e)) {
            // @ts-expect-error we use the ES2022 error CTOR option "cause" for better stack traces
            throw new Error(`cannot decode ${e.field()} from JSON: ${e.message}`, {
                cause: e,
            });
        }
        throw e;
    }
    return msg.message;
}
/**
 * Parse a message from a JSON value, merging fields.
 *
 * Repeated fields are appended. Map entries are added, overwriting
 * existing keys.
 *
 * If a message field is already present, it will be merged with the
 * new data.
 */
function mergeFromJson(schema, target, json, options) {
    try {
        readMessage(reflect(schema, target), json, makeReadOptions(options));
    }
    catch (e) {
        if (isFieldError(e)) {
            // @ts-expect-error we use the ES2022 error CTOR option "cause" for better stack traces
            throw new Error(`cannot decode ${e.field()} from JSON: ${e.message}`, {
                cause: e,
            });
        }
        throw e;
    }
    return target;
}
/**
 * Parses an enum value from JSON.
 */
function enumFromJson(descEnum, json) {
    return readEnum(descEnum, json, false);
}
/**
 * Is the given value a JSON enum value?
 */
function isEnumJson(descEnum, value) {
    return undefined !== descEnum.values.find((v) => v.name === value);
}
const messageJsonFields = new WeakMap();
function getJsonField(desc, jsonKey) {
    var _a;
    if (!messageJsonFields.has(desc)) {
        const jsonNames = new Map();
        for (const field of desc.fields) {
            jsonNames.set(field.name, field).set(field.jsonName, field);
        }
        messageJsonFields.set(desc, jsonNames);
    }
    return (_a = messageJsonFields.get(desc)) === null || _a === void 0 ? void 0 : _a.get(jsonKey);
}
function readMessage(msg, json, opts) {
    var _a;
    if (tryWktFromJson(msg, json, opts)) {
        return;
    }
    if (json == null || Array.isArray(json) || typeof json != "object") {
        throw new Error(`cannot decode ${msg.desc} from JSON: ${(0,reflect_check/* formatVal */.$L)(json)}`);
    }
    const oneofSeen = new Map();
    for (const [jsonKey, jsonValue] of Object.entries(json)) {
        const field = getJsonField(msg.desc, jsonKey);
        if (field) {
            if (field.oneof) {
                if (jsonValue === null && field.fieldKind == "scalar") {
                    // see conformance test Required.Proto3.JsonInput.OneofFieldNull{First,Second}
                    continue;
                }
                const seen = oneofSeen.get(field.oneof);
                if (seen !== undefined) {
                    throw new error/* FieldError */.b(field.oneof, `oneof set multiple times by ${seen.name} and ${field.name}`);
                }
                oneofSeen.set(field.oneof, field);
            }
            readField(msg, field, jsonValue, opts);
        }
        else {
            let extension = undefined;
            if (jsonKey.startsWith("[") &&
                jsonKey.endsWith("]") &&
                // biome-ignore lint/suspicious/noAssignInExpressions: no
                (extension = (_a = opts.registry) === null || _a === void 0 ? void 0 : _a.getExtension(jsonKey.substring(1, jsonKey.length - 1))) &&
                extension.extendee.typeName === msg.desc.typeName) {
                const [container, field, get] = (0,extensions/* createExtensionContainer */.z3)(extension);
                readField(container, field, jsonValue, opts);
                (0,extensions/* setExtension */.G6)(msg.message, extension, get());
            }
            if (!extension && !opts.ignoreUnknownFields) {
                throw new Error(`cannot decode ${msg.desc} from JSON: key "${jsonKey}" is unknown`);
            }
        }
    }
}
function readField(msg, field, json, opts) {
    switch (field.fieldKind) {
        case "scalar":
            readScalarField(msg, field, json);
            break;
        case "enum":
            readEnumField(msg, field, json, opts);
            break;
        case "message":
            readMessageField(msg, field, json, opts);
            break;
        case "list":
            readListField(msg.get(field), json, opts);
            break;
        case "map":
            readMapField(msg.get(field), json, opts);
            break;
    }
}
function readListOrMapItem(field, json, opts) {
    if (field.scalar && json !== null) {
        return scalarFromJson(field, json);
    }
    if (field.message && !isResetSentinelNullValue(field, json)) {
        const msgValue = (0,reflect_reflect/* reflect */.E4)(field.message);
        readMessage(msgValue, json, opts);
        return msgValue;
    }
    if (field.enum && !isResetSentinelNullValue(field, json)) {
        return readEnum(field.enum, json, opts.ignoreUnknownFields);
    }
    throw new error/* FieldError */.b(field, `${field.fieldKind === "list" ? "list item" : "map value"} must not be null`);
}
function readMapField(map, json, opts) {
    if (json === null) {
        return;
    }
    const field = map.field();
    if (typeof json != "object" || Array.isArray(json)) {
        throw new error/* FieldError */.b(field, "expected object, got " + (0,reflect_check/* formatVal */.$L)(json));
    }
    for (const [jsonMapKey, jsonMapValue] of Object.entries(json)) {
        const key = mapKeyFromJson(field.mapKey, jsonMapKey);
        const value = readListOrMapItem(field, jsonMapValue, opts);
        if (value !== tokenIgnoredUnknownEnum) {
            map.set(key, value);
        }
    }
}
function readListField(list, json, opts) {
    if (json === null) {
        return;
    }
    const field = list.field();
    if (!Array.isArray(json)) {
        throw new error/* FieldError */.b(field, "expected Array, got " + (0,reflect_check/* formatVal */.$L)(json));
    }
    for (const jsonItem of json) {
        const value = readListOrMapItem(field, jsonItem, opts);
        if (value !== tokenIgnoredUnknownEnum) {
            list.add(value);
        }
    }
}
function readMessageField(msg, field, json, opts) {
    if (isResetSentinelNullValue(field, json)) {
        msg.clear(field);
        return;
    }
    const msgValue = msg.isSet(field) ? msg.get(field) : (0,reflect_reflect/* reflect */.E4)(field.message);
    readMessage(msgValue, json, opts);
    msg.set(field, msgValue);
}
function readEnumField(msg, field, json, opts) {
    if (isResetSentinelNullValue(field, json)) {
        msg.clear(field);
        return;
    }
    const enumValue = readEnum(field.enum, json, opts.ignoreUnknownFields);
    if (enumValue !== tokenIgnoredUnknownEnum) {
        msg.set(field, enumValue);
    }
}
function readScalarField(msg, field, json) {
    if (json === null) {
        msg.clear(field);
    }
    else {
        msg.set(field, scalarFromJson(field, json));
    }
}
/**
 * Indicates whether a value is a sentinel for reseting a field.
 *
 * For this to be true, the value must be a JSON null and the field must not
 * permit a present, Protobuf-serializable null.
 *
 * Only message google.protobuf.Value and enum google.protobuf.NullValue fields
 * permit Protobuf-serializable nulls.
 *
 * Note that field-resetting sentinel nulls are not permitted in lists and maps.
 */
function isResetSentinelNullValue(field, json) {
    var _a, _b;
    return (json === null &&
        ((_a = field.message) === null || _a === void 0 ? void 0 : _a.typeName) != "google.protobuf.Value" &&
        ((_b = field.enum) === null || _b === void 0 ? void 0 : _b.typeName) != "google.protobuf.NullValue");
}
const tokenIgnoredUnknownEnum = Symbol();
function readEnum(desc, json, ignoreUnknownFields) {
    if (json === null) {
        return desc.values[0].number;
    }
    switch (typeof json) {
        case "number":
            if (Number.isInteger(json)) {
                return json;
            }
            break;
        case "string":
            const value = desc.values.find((ev) => ev.name === json);
            if (value !== undefined) {
                return value.number;
            }
            if (ignoreUnknownFields) {
                return tokenIgnoredUnknownEnum;
            }
            break;
    }
    throw new Error(`cannot decode ${desc} from JSON: ${(0,reflect_check/* formatVal */.$L)(json)}`);
}
/**
 * Try to parse a JSON value to a scalar value for the reflect API.
 *
 * Returns the input if the JSON value cannot be converted. Raises a FieldError
 * if conversion would be ambiguous.
 */
function scalarFromJson(field, json) {
    // int64, sfixed64, sint64, fixed64, uint64: Reflect supports string and number.
    // string, bool: Supported by reflect.
    switch (field.scalar) {
        // float, double: JSON value will be a number or one of the special string values "NaN", "Infinity", and "-Infinity".
        // Either numbers or strings are accepted. Exponent notation is also accepted.
        case descriptors/* ScalarType */.L.DOUBLE:
        case descriptors/* ScalarType */.L.FLOAT:
            if (json === "NaN")
                return NaN;
            if (json === "Infinity")
                return Number.POSITIVE_INFINITY;
            if (json === "-Infinity")
                return Number.NEGATIVE_INFINITY;
            if (typeof json == "number") {
                if (Number.isNaN(json)) {
                    // NaN must be encoded with string constants
                    throw new error/* FieldError */.b(field, "unexpected NaN number");
                }
                if (!Number.isFinite(json)) {
                    // Infinity must be encoded with string constants
                    throw new error/* FieldError */.b(field, "unexpected infinite number");
                }
                break;
            }
            if (typeof json == "string") {
                if (json === "") {
                    // empty string is not a number
                    break;
                }
                if (json.trim().length !== json.length) {
                    // extra whitespace
                    break;
                }
                const float = Number(json);
                if (!Number.isFinite(float)) {
                    // Infinity and NaN must be encoded with string constants
                    break;
                }
                return float;
            }
            break;
        // int32, fixed32, uint32: JSON value will be a decimal number. Either numbers or strings are accepted.
        case descriptors/* ScalarType */.L.INT32:
        case descriptors/* ScalarType */.L.FIXED32:
        case descriptors/* ScalarType */.L.SFIXED32:
        case descriptors/* ScalarType */.L.SINT32:
        case descriptors/* ScalarType */.L.UINT32:
            return int32FromJson(json);
        // bytes: JSON value will be the data encoded as a string using standard base64 encoding with paddings.
        // Either standard or URL-safe base64 encoding with/without paddings are accepted.
        case descriptors/* ScalarType */.L.BYTES:
            if (typeof json == "string") {
                if (json === "") {
                    return new Uint8Array(0);
                }
                try {
                    return (0,base64_encoding/* base64Decode */.u)(json);
                }
                catch (e) {
                    const message = e instanceof Error ? e.message : String(e);
                    throw new error/* FieldError */.b(field, message);
                }
            }
            break;
    }
    return json;
}
/**
 * Try to parse a JSON value to a map key for the reflect API.
 *
 * Returns the input if the JSON value cannot be converted.
 */
function mapKeyFromJson(type, jsonString) {
    switch (type) {
        case descriptors/* ScalarType */.L.BOOL:
            switch (jsonString) {
                case "true":
                    return true;
                case "false":
                    return false;
            }
            return jsonString;
        case descriptors/* ScalarType */.L.INT32:
        case descriptors/* ScalarType */.L.FIXED32:
        case descriptors/* ScalarType */.L.UINT32:
        case descriptors/* ScalarType */.L.SFIXED32:
        case descriptors/* ScalarType */.L.SINT32:
            return int32FromJson(jsonString);
        default:
            return jsonString;
    }
}
/**
 * Try to parse a JSON value to a 32-bit integer for the reflect API.
 *
 * Returns the input if the JSON value cannot be converted.
 */
function int32FromJson(json) {
    if (typeof json == "string") {
        if (json === "") {
            // empty string is not a number
            return json;
        }
        if (json.trim().length !== json.length) {
            // extra whitespace
            return json;
        }
        const num = Number(json);
        if (Number.isNaN(num)) {
            // not a number
            return json;
        }
        return num;
    }
    return json;
}
function parseJsonString(jsonString, typeName) {
    try {
        return JSON.parse(jsonString);
    }
    catch (e) {
        const message = e instanceof Error ? e.message : String(e);
        throw new Error(`cannot decode message ${typeName} from JSON: ${message}`, 
        // @ts-expect-error we use the ES2022 error CTOR option "cause" for better stack traces
        { cause: e });
    }
}
function tryWktFromJson(msg, jsonValue, opts) {
    if (!msg.desc.typeName.startsWith("google.protobuf.")) {
        return false;
    }
    switch (msg.desc.typeName) {
        case "google.protobuf.Any":
            anyFromJson(msg.message, jsonValue, opts);
            return true;
        case "google.protobuf.Timestamp":
            timestampFromJson(msg.message, jsonValue);
            return true;
        case "google.protobuf.Duration":
            durationFromJson(msg.message, jsonValue);
            return true;
        case "google.protobuf.FieldMask":
            fieldMaskFromJson(msg.message, jsonValue);
            return true;
        case "google.protobuf.Struct":
            structFromJson(msg.message, jsonValue);
            return true;
        case "google.protobuf.Value":
            valueFromJson(msg.message, jsonValue);
            return true;
        case "google.protobuf.ListValue":
            listValueFromJson(msg.message, jsonValue);
            return true;
        default:
            if ((0,wrappers/* isWrapperDesc */.l)(msg.desc)) {
                const valueField = msg.desc.fields[0];
                if (jsonValue === null) {
                    msg.clear(valueField);
                }
                else {
                    msg.set(valueField, scalarFromJson(valueField, jsonValue));
                }
                return true;
            }
            return false;
    }
}
function anyFromJson(any, json, opts) {
    var _a;
    if (json === null || Array.isArray(json) || typeof json != "object") {
        throw new Error(`cannot decode message ${any.$typeName} from JSON: expected object but got ${(0,reflect_check/* formatVal */.$L)(json)}`);
    }
    if (Object.keys(json).length == 0) {
        return;
    }
    const typeUrl = json["@type"];
    if (typeof typeUrl != "string" || typeUrl == "") {
        throw new Error(`cannot decode message ${any.$typeName} from JSON: "@type" is empty`);
    }
    const typeName = typeUrl.includes("/")
        ? typeUrl.substring(typeUrl.lastIndexOf("/") + 1)
        : typeUrl;
    if (!typeName.length) {
        throw new Error(`cannot decode message ${any.$typeName} from JSON: "@type" is invalid`);
    }
    const desc = (_a = opts.registry) === null || _a === void 0 ? void 0 : _a.getMessage(typeName);
    if (!desc) {
        throw new Error(`cannot decode message ${any.$typeName} from JSON: ${typeUrl} is not in the type registry`);
    }
    const msg = (0,reflect_reflect/* reflect */.E4)(desc);
    if (typeName.startsWith("google.protobuf.") &&
        Object.prototype.hasOwnProperty.call(json, "value")) {
        const value = json.value;
        readMessage(msg, value, opts);
    }
    else {
        const copy = Object.assign({}, json);
        // biome-ignore lint/performance/noDelete: <explanation>
        delete copy["@type"];
        readMessage(msg, copy, opts);
    }
    (0,wkt_any/* anyPack */.G0)(msg.desc, msg.message, any);
}
function timestampFromJson(timestamp, json) {
    if (typeof json !== "string") {
        throw new Error(`cannot decode message ${timestamp.$typeName} from JSON: ${(0,reflect_check/* formatVal */.$L)(json)}`);
    }
    const matches = json.match(/^([0-9]{4})-([0-9]{2})-([0-9]{2})T([0-9]{2}):([0-9]{2}):([0-9]{2})(?:\.([0-9]{1,9}))?(?:Z|([+-][0-9][0-9]:[0-9][0-9]))$/);
    if (!matches) {
        throw new Error(`cannot decode message ${timestamp.$typeName} from JSON: invalid RFC 3339 string`);
    }
    const ms = Date.parse(
    // biome-ignore format: want this to read well
    matches[1] + "-" + matches[2] + "-" + matches[3] + "T" + matches[4] + ":" + matches[5] + ":" + matches[6] + (matches[8] ? matches[8] : "Z"));
    if (Number.isNaN(ms)) {
        throw new Error(`cannot decode message ${timestamp.$typeName} from JSON: invalid RFC 3339 string`);
    }
    if (ms < Date.parse("0001-01-01T00:00:00Z") ||
        ms > Date.parse("9999-12-31T23:59:59Z")) {
        throw new Error(`cannot decode message ${timestamp.$typeName} from JSON: must be from 0001-01-01T00:00:00Z to 9999-12-31T23:59:59Z inclusive`);
    }
    timestamp.seconds = proto_int64/* protoInt64 */.M.parse(ms / 1000);
    timestamp.nanos = 0;
    if (matches[7]) {
        timestamp.nanos =
            parseInt("1" + matches[7] + "0".repeat(9 - matches[7].length)) -
                1000000000;
    }
}
function durationFromJson(duration, json) {
    if (typeof json !== "string") {
        throw new Error(`cannot decode message ${duration.$typeName} from JSON: ${(0,reflect_check/* formatVal */.$L)(json)}`);
    }
    const match = json.match(/^(-?[0-9]+)(?:\.([0-9]+))?s/);
    if (match === null) {
        throw new Error(`cannot decode message ${duration.$typeName} from JSON: ${(0,reflect_check/* formatVal */.$L)(json)}`);
    }
    const longSeconds = Number(match[1]);
    if (longSeconds > 315576000000 || longSeconds < -315576000000) {
        throw new Error(`cannot decode message ${duration.$typeName} from JSON: ${(0,reflect_check/* formatVal */.$L)(json)}`);
    }
    duration.seconds = proto_int64/* protoInt64 */.M.parse(longSeconds);
    if (typeof match[2] !== "string") {
        return;
    }
    const nanosStr = match[2] + "0".repeat(9 - match[2].length);
    duration.nanos = parseInt(nanosStr);
    if (longSeconds < 0 || Object.is(longSeconds, -0)) {
        duration.nanos = -duration.nanos;
    }
}
function fieldMaskFromJson(fieldMask, json) {
    if (typeof json !== "string") {
        throw new Error(`cannot decode message ${fieldMask.$typeName} from JSON: ${(0,reflect_check/* formatVal */.$L)(json)}`);
    }
    if (json === "") {
        return;
    }
    fieldMask.paths = json.split(",").map((path) => {
        if (path.includes("_")) {
            throw new Error(`cannot decode message ${fieldMask.$typeName} from JSON: path names must be lowerCamelCase`);
        }
        return (0,names/* protoSnakeCase */.t5)(path);
    });
}
function structFromJson(struct, json) {
    if (typeof json != "object" || json == null || Array.isArray(json)) {
        throw new Error(`cannot decode message ${struct.$typeName} from JSON ${(0,reflect_check/* formatVal */.$L)(json)}`);
    }
    for (const [k, v] of Object.entries(json)) {
        const parsedV = (0,create/* create */.v)(ValueSchema);
        valueFromJson(parsedV, v);
        struct.fields[k] = parsedV;
    }
}
function valueFromJson(value, json) {
    switch (typeof json) {
        case "number":
            value.kind = { case: "numberValue", value: json };
            break;
        case "string":
            value.kind = { case: "stringValue", value: json };
            break;
        case "boolean":
            value.kind = { case: "boolValue", value: json };
            break;
        case "object":
            if (json === null) {
                value.kind = { case: "nullValue", value: NullValue.NULL_VALUE };
            }
            else if (Array.isArray(json)) {
                const listValue = (0,create/* create */.v)(ListValueSchema);
                listValueFromJson(listValue, json);
                value.kind = { case: "listValue", value: listValue };
            }
            else {
                const struct = (0,create/* create */.v)(StructSchema);
                structFromJson(struct, json);
                value.kind = { case: "structValue", value: struct };
            }
            break;
        default:
            throw new Error(`cannot decode message ${value.$typeName} from JSON ${(0,reflect_check/* formatVal */.$L)(json)}`);
    }
    return value;
}
function listValueFromJson(listValue, json) {
    if (!Array.isArray(json)) {
        throw new Error(`cannot decode message ${listValue.$typeName} from JSON ${(0,reflect_check/* formatVal */.$L)(json)}`);
    }
    for (const e of json) {
        const value = (0,create/* create */.v)(ValueSchema);
        valueFromJson(value, e);
        listValue.values.push(value);
    }
}


/***/ }),

/***/ 95765:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Z: () => (/* binding */ serviceDesc)
/* harmony export */ });
// Copyright 2021-2026 Buf Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
/**
 * Hydrate a service descriptor.
 *
 * @private
 */
function serviceDesc(file, path, ...paths) {
    if (paths.length > 0) {
        throw new Error();
    }
    return file.services[path];
}


/***/ }),

/***/ 96966:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Nn: () => (/* binding */ errorFromJson)
/* harmony export */ });
/* unused harmony exports errorFromJsonBytes, errorToJson, errorToJsonBytes */
/* harmony import */ var _bufbuild_protobuf_wire__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(14097);
/* harmony import */ var _connect_error_js__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(45640);
/* harmony import */ var _code_string_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(12386);
// Copyright 2021-2025 The Connect Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
var __rest = (undefined && undefined.__rest) || function (s, e) {
    var t = {};
    for (var p in s) if (Object.prototype.hasOwnProperty.call(s, p) && e.indexOf(p) < 0)
        t[p] = s[p];
    if (s != null && typeof Object.getOwnPropertySymbols === "function")
        for (var i = 0, p = Object.getOwnPropertySymbols(s); i < p.length; i++) {
            if (e.indexOf(p[i]) < 0 && Object.prototype.propertyIsEnumerable.call(s, p[i]))
                t[p[i]] = s[p[i]];
        }
    return t;
};





/**
 * Parse a Connect error from a JSON value.
 * Will return a ConnectError, and throw the provided fallback if parsing failed.
 *
 * @private Internal code, does not follow semantic versioning.
 */
function errorFromJson(jsonValue, metadata, fallback) {
    var _a;
    if (metadata) {
        new Headers(metadata).forEach((value, key) => fallback.metadata.append(key, value));
    }
    if (typeof jsonValue !== "object" ||
        jsonValue == null ||
        Array.isArray(jsonValue)) {
        throw fallback;
    }
    let code = fallback.code;
    if ("code" in jsonValue && typeof jsonValue.code === "string") {
        code = (_a = (0,_code_string_js__WEBPACK_IMPORTED_MODULE_0__/* .codeFromString */ .X)(jsonValue.code)) !== null && _a !== void 0 ? _a : code;
    }
    const message = jsonValue.message;
    if (message != null && typeof message !== "string") {
        throw fallback;
    }
    const error = new _connect_error_js__WEBPACK_IMPORTED_MODULE_1__/* .ConnectError */ .T(message !== null && message !== void 0 ? message : "", code, metadata);
    if ("details" in jsonValue && Array.isArray(jsonValue.details)) {
        for (const detail of jsonValue.details) {
            if (detail === null ||
                typeof detail != "object" ||
                Array.isArray(detail) ||
                typeof detail.type != "string" ||
                typeof detail.value != "string") {
                throw fallback;
            }
            try {
                error.details.push({
                    type: detail.type,
                    value: (0,_bufbuild_protobuf_wire__WEBPACK_IMPORTED_MODULE_2__/* .base64Decode */ .u)(detail.value),
                    debug: detail.debug,
                });
            }
            catch (e) {
                throw fallback;
            }
        }
    }
    return error;
}
/**
 * Parse a Connect error from a serialized JSON value.
 * Will return a ConnectError, and throw the provided fallback if parsing failed.
 *
 * @private Internal code, does not follow semantic versioning.
 */
function errorFromJsonBytes(bytes, metadata, fallback) {
    let jsonValue;
    try {
        jsonValue = JSON.parse(new TextDecoder().decode(bytes));
    }
    catch (e) {
        throw fallback;
    }
    return errorFromJson(jsonValue, metadata, fallback);
}
/**
 * Serialize the given error to JSON.
 *
 * The JSON serialization options are required to produce the optional
 * human-readable representation in the "debug" key if the detail uses
 * google.protobuf.Any. If serialization of the "debug" value fails, it
 * is silently disregarded.
 *
 * See https://connectrpc.com/docs/protocol#error-end-stream
 *
 * @private Internal code, does not follow semantic versioning.
 */
function errorToJson(error, jsonWriteOptions) {
    const o = {
        code: codeToString(error.code),
    };
    if (error.rawMessage.length > 0) {
        o.message = error.rawMessage;
    }
    if (error.details.length > 0) {
        o.details = error.details
            .map((detail) => {
            if ("desc" in detail) {
                const msg = create(detail.desc, detail.value);
                const i = {
                    type: detail.desc.typeName,
                    value: toBinary(detail.desc, msg),
                };
                try {
                    i.debug = toJson(detail.desc, msg, jsonWriteOptions);
                }
                catch (e) {
                    // We deliberately ignore errors that may occur when serializing
                    // a message to JSON (the message contains an Any).
                    // The rationale is that we are only trying to provide optional
                    // debug information.
                }
                return i;
            }
            return detail;
        })
            .map((_a) => {
            var { value } = _a, rest = __rest(_a, ["value"]);
            return (Object.assign(Object.assign({}, rest), { value: base64Encode(value, "std_raw") }));
        });
    }
    return o;
}
/**
 * Serialize the given error to JSON. This calls errorToJson(), but stringifies
 * the result, and converts it into a UInt8Array.
 *
 * @private Internal code, does not follow semantic versioning.
 */
function errorToJsonBytes(error, jsonWriteOptions) {
    const textEncoder = new TextEncoder();
    try {
        const jsonObject = errorToJson(error, jsonWriteOptions);
        const jsonString = JSON.stringify(jsonObject);
        return textEncoder.encode(jsonString);
    }
    catch (e) {
        const m = e instanceof Error ? e.message : String(e);
        throw new ConnectError(`failed to serialize Connect Error: ${m}`, Code.Internal);
    }
}


/***/ })

}]);
//# sourceMappingURL=9353-e21245d51f178ca5.js.map