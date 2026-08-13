import { createSlice, PayloadAction } from '@reduxjs/toolkit';

interface BulkSelectionState {
  selectedIds: string[];
}

const initialState: BulkSelectionState = {
  selectedIds: [],
};

const bulkSelectionSlice = createSlice({
  name: 'bulkSelection',
  initialState,
  reducers: {
    toggleSelection(state, action: PayloadAction<string>) {
      const id = action.payload;
      const idx = state.selectedIds.indexOf(id);
      if (idx >= 0) {
        state.selectedIds.splice(idx, 1);
      } else {
        state.selectedIds.push(id);
      }
    },
    selectAll(state, action: PayloadAction<string[]>) {
      state.selectedIds = action.payload;
    },
    clearSelection(state) {
      state.selectedIds = [];
    },
  },
});

export const { toggleSelection, selectAll, clearSelection } = bulkSelectionSlice.actions;
export default bulkSelectionSlice.reducer;
