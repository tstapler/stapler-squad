/**
 * Standalone test script for envelope encoding
 * Run with: node test-envelope.js
 */

function encodeEnvelope(flags, data) {
  const envelope = new Uint8Array(5 + data.length);
  envelope[0] = flags;
  // Write length as big-endian uint32
  const view = new DataView(envelope.buffer, envelope.byteOffset, envelope.byteLength);
  view.setUint32(1, data.length, false); // false = big-endian
  envelope.set(data, 5);
  return envelope;
}

function assert(condition, message) {
  if (!condition) {
    throw new Error(`Assertion failed: ${message}`);
  }
}

function arrayEquals(a, b) {
  if (a.length !== b.length) return false;
  for (let i = 0; i < a.length; i++) {
    if (a[i] !== b[i]) return false;
  }
  return true;
}

console.log('Running envelope encoding tests...\n');

// Test 1: Simple message
console.log('Test 1: Simple message');
{
  const data = new TextEncoder().encode('test');
  const envelope = encodeEnvelope(0x00, data);
  const expected = new Uint8Array([0x00, 0x00, 0x00, 0x00, 0x04, 0x74, 0x65, 0x73, 0x74]);
  assert(arrayEquals(envelope, expected), 'Simple message encoding failed');
  console.log('✓ Passed');
}

// Test 2: Empty data
console.log('Test 2: Empty data');
{
  const envelope = encodeEnvelope(0x00, new Uint8Array(0));
  const expected = new Uint8Array([0x00, 0x00, 0x00, 0x00, 0x00]);
  assert(arrayEquals(envelope, expected), 'Empty data encoding failed');
  console.log('✓ Passed');
}

// Test 3: End stream flag
console.log('Test 3: End stream flag');
{
  const data = new TextEncoder().encode('end');
  const envelope = encodeEnvelope(0x02, data);
  assert(envelope[0] === 0x02, 'Flag not set correctly');
  assert(envelope[4] === 0x03, 'Length not correct');
  console.log('✓ Passed');
}

// Test 4: Big-endian encoding (256 bytes)
console.log('Test 4: Big-endian encoding (256 bytes)');
{
  const data = new Uint8Array(256);
  const envelope = encodeEnvelope(0x00, data);
  // 256 = 0x00000100 in big-endian
  assert(envelope[1] === 0x00, 'Byte 1 incorrect');
  assert(envelope[2] === 0x00, 'Byte 2 incorrect');
  assert(envelope[3] === 0x01, 'Byte 3 incorrect');
  assert(envelope[4] === 0x00, 'Byte 4 incorrect');
  console.log('✓ Passed');
}

// Test 5: Protobuf-like data
console.log('Test 5: Protobuf-like data');
{
  const data = new Uint8Array([
    0x0a, 0x0b, 0x6e, 0x65, 0x77, 0x2d, 0x73, 0x65, 0x73, 0x73, 0x69, 0x6f, 0x6e,
    0x1a, 0x03, 0x0a, 0x01, 0x74
  ]);
  const envelope = encodeEnvelope(0x00, data);
  assert(envelope[0] === 0x00, 'Flags incorrect');
  assert(envelope[4] === data.length, 'Length incorrect');
  assert(arrayEquals(envelope.slice(5), data), 'Data not preserved');
  console.log('✓ Passed');
}

// Test 6: Match Go CreateEnvelope output
console.log('Test 6: Match Go CreateEnvelope output');
{
  const data = new TextEncoder().encode('hello');
  const envelope = encodeEnvelope(0x00, data);
  const expected = new Uint8Array([
    0x00, 0x00, 0x00, 0x00, 0x05,
    0x68, 0x65, 0x6c, 0x6c, 0x6f
  ]);
  assert(arrayEquals(envelope, expected), 'Go compatibility failed');
  console.log('✓ Passed');
  console.log(`  Hex: ${Array.from(envelope).map(b => b.toString(16).padStart(2, '0')).join('')}`);
}

// Test 7: DataView byteOffset handling
console.log('Test 7: DataView byteOffset handling');
{
  const data = new Uint8Array([0xAA, 0xBB]);
  const envelope = encodeEnvelope(0x00, data);
  const view = new DataView(envelope.buffer, envelope.byteOffset + 1, 4);
  const length = view.getUint32(0, false);
  assert(length === 2, 'DataView length reading failed');
  assert(envelope[5] === 0xAA, 'First data byte incorrect');
  assert(envelope[6] === 0xBB, 'Second data byte incorrect');
  console.log('✓ Passed');
}

console.log('\n✅ All tests passed!');
