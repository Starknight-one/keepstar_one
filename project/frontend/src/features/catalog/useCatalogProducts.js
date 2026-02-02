import { useState, useCallback } from 'react';
import { getProducts } from '../../shared/api/apiClient';
import { createInitialCatalogState } from './catalogModel';

export function useCatalogProducts(tenantSlug) {
  const [state, setState] = useState(createInitialCatalogState);

  const loadProducts = useCallback(async (filters = {}) => {
    setState(s => ({ ...s, isLoading: true, error: null }));

    try {
      const mergedFilters = { ...state.filters, ...filters };
      const data = await getProducts(tenantSlug, mergedFilters);

      setState(s => ({
        ...s,
        products: data.products || [],
        total: data.total || 0,
        filters: mergedFilters,
        isLoading: false,
      }));
    } catch (err) {
      setState(s => ({
        ...s,
        error: err.message,
        isLoading: false,
      }));
    }
  }, [tenantSlug, state.filters]);

  const setFilters = useCallback((newFilters) => {
    setState(s => ({
      ...s,
      filters: { ...s.filters, ...newFilters },
    }));
  }, []);

  const resetFilters = useCallback(() => {
    setState(s => ({
      ...s,
      filters: createInitialCatalogState().filters,
    }));
  }, []);

  return {
    products: state.products,
    total: state.total,
    isLoading: state.isLoading,
    error: state.error,
    filters: state.filters,
    loadProducts,
    setFilters,
    resetFilters,
  };
}
