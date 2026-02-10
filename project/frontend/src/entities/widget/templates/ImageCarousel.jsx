// Shared image carousel for card templates (ProductCard, ServiceCard)
export function ImageCarousel({ images, currentIndex, onIndexChange }) {
  if (!images || images.length === 0) return null;

  const handleImageClick = () => {
    if (images.length > 1) {
      onIndexChange((currentIndex + 1) % images.length);
    }
  };

  return (
    <div className="image-carousel">
      <img
        src={images[currentIndex]}
        alt=""
        className="carousel-image"
        onClick={handleImageClick}
      />
      {images.length > 1 && (
        <div className="carousel-dots">
          {images.map((_, index) => (
            <button
              key={index}
              className={`carousel-dot ${index === currentIndex ? 'active' : ''}`}
              onClick={(e) => {
                e.stopPropagation();
                onIndexChange(index);
              }}
            />
          ))}
        </div>
      )}
    </div>
  );
}
