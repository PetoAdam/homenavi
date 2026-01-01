import React, { useEffect, useMemo, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import './HoverDescription.css';

export default function HoverDescription({
  title,
  description,
  children,
  disabled = false,
  placement = 'top',
}) {
  const anchorRef = useRef(null);
  const [open, setOpen] = useState(false);
  const [pos, setPos] = useState({ x: 0, y: 0, w: 0, h: 0 });

  const hasContent = Boolean((title && String(title).trim()) || (description && String(description).trim()));

  const updatePosition = () => {
    const el = anchorRef.current;
    if (!el) return;
    const rect = el.getBoundingClientRect();
    setPos({ x: rect.left, y: rect.top, w: rect.width, h: rect.height });
  };

  useEffect(() => {
    if (!open) return;
    updatePosition();
    const onScroll = () => updatePosition();
    const onResize = () => updatePosition();
    window.addEventListener('scroll', onScroll, true);
    window.addEventListener('resize', onResize);
    return () => {
      window.removeEventListener('scroll', onScroll, true);
      window.removeEventListener('resize', onResize);
    };
  }, [open]);

  const style = useMemo(() => {
    const gap = 10;
    const cx = pos.x + pos.w / 2;
    const top = pos.y;
    const bottom = pos.y + pos.h;

    if (placement === 'bottom') {
      return {
        left: cx,
        top: bottom + gap,
        transform: 'translateX(-50%)',
      };
    }

    return {
      left: cx,
      top: top - gap,
      transform: 'translate(-50%, -100%)',
    };
  }, [placement, pos.h, pos.w, pos.x, pos.y]);

  return (
    <span
      className="hover-desc-anchor"
      ref={anchorRef}
      onMouseEnter={() => {
        if (disabled || !hasContent) return;
        setOpen(true);
      }}
      onMouseLeave={() => setOpen(false)}
      onFocus={() => {
        if (disabled || !hasContent) return;
        setOpen(true);
      }}
      onBlur={() => setOpen(false)}
    >
      {children}
      {open && hasContent ? createPortal(
        <div className="hover-desc-pop" style={style} role="tooltip">
          {title ? <div className="hover-desc-title">{title}</div> : null}
          {description ? <div className="hover-desc-body">{description}</div> : null}
        </div>,
        document.body,
      ) : null}
    </span>
  );
}
