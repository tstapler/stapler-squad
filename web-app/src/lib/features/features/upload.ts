import type { Feature } from '../types';

export const uploadFeatures = {
  'upload-file': {
    id: 'upload-file',
    title: 'Upload File',
    description: 'Handles file uploads via HTTP POST to the upload endpoint.',
    rpcIds: ['upload:file'],
    componentPaths: [],
    testIds: [],
    status: 'experimental',
    since: '1.0.0',
  },
  'upload-image': {
    id: 'upload-image',
    title: 'Upload Image',
    description: 'Handles image uploads via HTTP POST, supporting JPEG, PNG, and WebP formats.',
    rpcIds: ['upload:image'],
    componentPaths: [],
    testIds: [
      'TestSessionImageUpload_Success_JPEG',
      'TestSessionImageUpload_Success_PNG',
      'TestSessionImageUpload_Success_WebP',
      'TestSessionImageUpload_OversizedFile',
      'TestSessionImageUpload_InvalidMIMEType',
      'TestSessionImageUpload_EmptyFile',
      'TestSessionImageUpload_SessionNotFound',
      'TestSessionImageUpload_MissingSessionID',
      'TestSessionImageUpload_MissingFile',
      'TestSessionImageUpload_PathTraversal',
      'TestSessionImageUpload_SessionNoPath',
      'TestSessionImageUpload_SessionPathMissing',
      'TestSessionImageUpload_ConcurrentUploads',
    ],
    status: 'stable',
    since: '1.0.0',
  },
} as const satisfies Record<string, Feature>;
