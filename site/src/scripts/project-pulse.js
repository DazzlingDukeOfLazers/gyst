export function createCardHoverSync({
  eventTrack,
  resolveDayAtPoint,
  onDayChange,
  requestFrame = window.requestAnimationFrame.bind(window),
  cancelFrame = window.cancelAnimationFrame.bind(window),
}) {
  let pointer = null;
  let pendingFrame = null;
  let activeDay = null;

  const syncDayUnderPointer = () => {
    pendingFrame = null;
    if (!pointer) return;
    const day = resolveDayAtPoint(pointer.x, pointer.y);
    if (day !== null) {
      activeDay = day;
      onDayChange(day);
    }
  };

  const scheduleSync = () => {
    if (!pointer || pendingFrame !== null) return;
    pendingFrame = -1;
    const frame = requestFrame(syncDayUnderPointer);
    if (pendingFrame !== null) pendingFrame = frame;
  };

  const rememberPointer = (event) => {
    pointer = { x: event.clientX, y: event.clientY };
    scheduleSync();
  };

  const forgetPointer = () => {
    pointer = null;
    activeDay = null;
    if (pendingFrame !== null) cancelFrame(pendingFrame);
    pendingFrame = null;
  };

  eventTrack.addEventListener('pointerover', rememberPointer);
  eventTrack.addEventListener('pointermove', rememberPointer);
  eventTrack.addEventListener('pointerleave', forgetPointer);
  eventTrack.addEventListener('scroll', scheduleSync, { passive: true });

  return {
    getActiveDay() {
      return activeDay;
    },
    destroy() {
      forgetPointer();
      eventTrack.removeEventListener('pointerover', rememberPointer);
      eventTrack.removeEventListener('pointermove', rememberPointer);
      eventTrack.removeEventListener('pointerleave', forgetPointer);
      eventTrack.removeEventListener('scroll', scheduleSync);
    },
  };
}
