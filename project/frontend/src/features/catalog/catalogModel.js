// Catalog state model

export function createInitialCatalogState() {
  return {
    products: [],
    total: 0,
    isLoading: false,
    error: null,
    filters: {
      category: null,
      brand: null,
      search: '',
      minPrice: null,
      maxPrice: null,
      limit: 20,
      offset: 0,
    },
  };
}
