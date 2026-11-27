// Bridges the UMD React global to the bare "react" specifier used by the
// component sources, so UI kits can import them directly in the browser.
const R = window.React;
export default R;
export const {
  useState, useEffect, useRef, useMemo, useCallback, useReducer, useContext,
  createElement, cloneElement, Fragment, createContext, forwardRef, memo, Children, isValidElement,
} = R;
