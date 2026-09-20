import assert from 'node:assert/strict';
import test from 'node:test';

import { createCardHoverSync } from '../src/scripts/project-pulse.js';

function pointerEvent(type, clientX = 80, clientY = 120) {
  const event = new Event(type);
  Object.defineProperties(event, {
    clientX: { value: clientX },
    clientY: { value: clientY },
  });
  return event;
}

function immediateFrame(callback) {
  callback();
  return 1;
}

test('updates the focused day when scrolling puts a neighboring card under a stationary pointer', () => {
  const eventTrack = new EventTarget();
  const focusedDays = [];
  let dayUnderPointer = '2';

  const controller = createCardHoverSync({
    eventTrack,
    resolveDayAtPoint: () => dayUnderPointer,
    onDayChange: (day) => focusedDays.push(day),
    requestFrame: immediateFrame,
    cancelFrame: () => {},
  });

  eventTrack.dispatchEvent(pointerEvent('pointerover'));
  dayUnderPointer = '3';
  eventTrack.dispatchEvent(new Event('scroll'));

  assert.deepEqual(focusedDays, ['2', '3']);
  controller.destroy();
});

test('keeps the current day through the gap between cards, then activates the neighbor', () => {
  const eventTrack = new EventTarget();
  const focusedDays = [];
  let dayUnderPointer = '4';

  const controller = createCardHoverSync({
    eventTrack,
    resolveDayAtPoint: () => dayUnderPointer,
    onDayChange: (day) => focusedDays.push(day),
    requestFrame: immediateFrame,
    cancelFrame: () => {},
  });

  eventTrack.dispatchEvent(pointerEvent('pointermove'));
  dayUnderPointer = null;
  eventTrack.dispatchEvent(new Event('scroll'));
  assert.equal(controller.getActiveDay(), '4');
  dayUnderPointer = '5';
  eventTrack.dispatchEvent(new Event('scroll'));

  assert.deepEqual(focusedDays, ['4', '5']);
  assert.equal(controller.getActiveDay(), '5');
  controller.destroy();
});

test('stops reacting to scroll after the pointer leaves the card list', () => {
  const eventTrack = new EventTarget();
  const focusedDays = [];
  let dayUnderPointer = '6';

  const controller = createCardHoverSync({
    eventTrack,
    resolveDayAtPoint: () => dayUnderPointer,
    onDayChange: (day) => focusedDays.push(day),
    requestFrame: immediateFrame,
    cancelFrame: () => {},
  });

  eventTrack.dispatchEvent(pointerEvent('pointerover'));
  eventTrack.dispatchEvent(pointerEvent('pointerleave'));
  assert.equal(controller.getActiveDay(), null);
  dayUnderPointer = '7';
  eventTrack.dispatchEvent(new Event('scroll'));

  assert.deepEqual(focusedDays, ['6']);
  controller.destroy();
});
