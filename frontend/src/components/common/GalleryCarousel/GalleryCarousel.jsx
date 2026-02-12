import React, { useCallback, useEffect, useMemo, useState } from 'react';

export default function GalleryCarousel({ images, onOpen }) {
  const galleryImages = useMemo(() => (
    Array.isArray(images) ? images.filter(Boolean) : []
  ), [images]);
  const [galleryIndex, setGalleryIndex] = useState(0);
  const [galleryOpen, setGalleryOpen] = useState(false);
  const [displaySrc, setDisplaySrc] = useState('');
  const hasGallery = galleryImages.length > 0;

  const openGallery = useCallback((index) => {
    setGalleryIndex(index);
    setGalleryOpen(true);
    if (onOpen) {
      onOpen(index);
    }
  }, [onOpen]);

  const closeGallery = useCallback(() => {
    setGalleryOpen(false);
  }, []);

  const showPrev = useCallback(() => {
    setGalleryIndex((prev) => (prev - 1 + galleryImages.length) % galleryImages.length);
  }, [galleryImages.length]);

  const showNext = useCallback(() => {
    setGalleryIndex((prev) => (prev + 1) % galleryImages.length);
  }, [galleryImages.length]);

  useEffect(() => {
    if (!galleryOpen || !hasGallery) return undefined;
    const nextSrc = galleryImages[galleryIndex];
    let cancelled = false;
    setDisplaySrc('');
    const img = new Image();
    img.onload = () => {
      if (!cancelled) {
        setDisplaySrc(nextSrc);
      }
    };
    img.onerror = () => {
      if (!cancelled) {
        setDisplaySrc(nextSrc);
      }
    };
    img.src = nextSrc;
    return () => {
      cancelled = true;
    };
  }, [galleryIndex, galleryImages, galleryOpen, hasGallery]);

  useEffect(() => {
    if (!galleryOpen) {
      setDisplaySrc('');
    }
  }, [galleryOpen]);

  useEffect(() => {
    if (!galleryOpen) return undefined;
    const handleKeyDown = (event) => {
      if (event.key === 'Escape') {
        event.preventDefault();
        closeGallery();
      } else if (event.key === 'ArrowLeft') {
        event.preventDefault();
        showPrev();
      } else if (event.key === 'ArrowRight') {
        event.preventDefault();
        showNext();
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [galleryOpen, closeGallery, showNext, showPrev]);

  if (!hasGallery) return null;

  return (
    <>
      <div className="integrations-admin-marketplace-gallery">
        {galleryImages.map((src, index) => (
          <button
            key={src}
            type="button"
            className="integrations-admin-marketplace-gallery-item"
            onClick={() => openGallery(index)}
          >
            <img src={src} alt="" />
          </button>
        ))}
      </div>
      {galleryOpen ? (
        <div className="integrations-admin-gallery-modal" onClick={closeGallery}>
          <div className="integrations-admin-gallery-dialog" onClick={(event) => event.stopPropagation()}>
            <button type="button" className="integrations-admin-gallery-close" onClick={closeGallery}>×</button>
            <button type="button" className="integrations-admin-gallery-nav prev" onClick={showPrev}>
              ‹
            </button>
            {displaySrc ? (
              <img src={displaySrc} alt="" className="integrations-admin-gallery-image" />
            ) : (
              <div className="integrations-admin-gallery-image placeholder" />
            )}
            <button type="button" className="integrations-admin-gallery-nav next" onClick={showNext}>
              ›
            </button>
            <div className="integrations-admin-gallery-count">
              {galleryIndex + 1} / {galleryImages.length}
            </div>
          </div>
        </div>
      ) : null}
    </>
  );
}
