import { createSlice, PayloadAction } from "@reduxjs/toolkit";

interface BacklogBulkSelectionState {
  selectedIds: string[];
  selectMode: boolean;
}

const initialState: BacklogBulkSelectionState = {
  selectedIds: [],
  selectMode: false,
};

// Separate from bulkSelectionSlice (sessions) so selecting backlog rows never
// touches session selection.
const backlogBulkSelectionSlice = createSlice({
  name: "backlogBulkSelection",
  initialState,
  reducers: {
    toggleSelection(state, action: PayloadAction<string>) {
      const idx = state.selectedIds.indexOf(action.payload);
      if (idx >= 0) {
        state.selectedIds.splice(idx, 1);
      } else {
        state.selectedIds.push(action.payload);
      }
      state.selectMode = true;
    },
    selectAll(state, action: PayloadAction<string[]>) {
      state.selectedIds = action.payload;
      state.selectMode = action.payload.length > 0;
    },
    clearSelection(state) {
      state.selectedIds = [];
      state.selectMode = false;
    },
  },
});

export const { toggleSelection, selectAll, clearSelection } = backlogBulkSelectionSlice.actions;

/** Action to apply after a bulk fan-out: clear on full success, otherwise keep only the failed ids selected. */
export const selectionAfterBulk = (failedIds: string[]) =>
  failedIds.length === 0 ? clearSelection() : selectAll(failedIds);

export default backlogBulkSelectionSlice.reducer;
