import { mkdir, writeFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import path from 'node:path'

// Original, procedurally composed instrumental bed: no sampled or downloaded music.
export async function createSoundtrack(destination) {
  const rate = 48000, duration = 36, count = rate * duration;
  const left = new Float32Array(count), right = new Float32Array(count);
  const beat = .5625, tau = Math.PI * 2;
  let randomState = 9241;
  const noise = () => { randomState = (Math.imul(randomState, 1664525) + 1013904223) >>> 0; return randomState / 2147483648 - 1; };
  function tone(start, length, hz, gain, pan = 0, kind = 'bell') {
    const begin = Math.round(start * rate), samples = Math.round(length * rate);
    const l = Math.sqrt((1 - pan) * .5), r = Math.sqrt((1 + pan) * .5);
    for (let n = 0; n < samples && begin + n < count; n++) {
      const t = n / rate, p = t / length;
      const attack = Math.min(1, t / (kind === 'pad' ? .35 : .014));
      const envelope = kind === 'pad' ? attack * Math.min(1, (length - t) / .6) * .55 : attack * Math.exp(-t * (kind === 'bass' ? 5 : 3.5)) * (1 - p);
      const value = kind === 'pad'
        ? (Math.sin(tau * hz * t) + .24 * Math.sin(tau * hz * 2.002 * t) + .13 * Math.sin(tau * hz * .998 * t)) * envelope * gain
        : (Math.sin(tau * hz * t) + (kind === 'bass' ? .13 : .3) * Math.sin(tau * hz * 2 * t)) * envelope * gain;
      left[begin + n] += value * l; right[begin + n] += value * r;
    }
  }
  const chords = [[146.8324,174.6141,220],[174.6141,220,261.6256],[130.8128,164.8138,195.9977],[116.5409,146.8324,174.6141]];
  for (let bar = 0; bar < 16; bar++) {
    const start = bar * beat * 4, chord = chords[bar % chords.length];
    chord.forEach((hz, index) => tone(start, beat * 4 + .55, hz * 2, .045, (index - 1) * .55, 'pad'));
    for (const offset of [0, 2, 3.5]) tone(start + offset * beat, .8, chord[0] / 2, bar < 2 ? .035 : .055, 0, 'bass');
    if (bar > 1 && bar < 15) {
      for (const offset of [0, 2]) {
        const begin = Math.round((start + offset * beat) * rate);
        for (let n = 0; n < rate * .24 && begin + n < count; n++) {
          const t = n / rate;
          const v = Math.sin(tau * (47 * t + 3.1 * (1 - Math.exp(-22 * t)))) * Math.exp(-18 * t) * .065;
          left[begin + n] += v; right[begin + n] += v;
        }
      }
      for (const offset of [1, 3]) {
        const begin = Math.round((start + offset * beat) * rate);
        for (let n = 0; n < rate * .13 && begin + n < count; n++) {
          const t = n / rate, v = noise() * Math.exp(-45 * t) * .014;
          left[begin + n] += v; right[begin + n] += v;
        }
      }
      for (const offset of [.5, 1.5, 2.5, 3.5]) {
        const begin = Math.round((start + offset * beat) * rate);
        let prev = 0;
        for (let n = 0; n < rate * .045 && begin + n < count; n++) {
          const sample = noise(), t = n / rate, v = (sample - prev) * Math.exp(-95 * t) * .005;
          prev = sample; left[begin + n] += v * .7; right[begin + n] += v;
        }
      }
    }
    if (bar % 2 === 0) {
      tone(start + beat * .5, 1.1, chord[2] * 4, .035, -.3);
      tone(start + beat * 2, .9, chord[1] * 4, .028, .35);
    }
  }
  for (const cue of [4.3, 10.1, 17.5, 24.2, 27.35, 31.5]) {
    tone(cue, .85, 880, .025, -.25); tone(cue + .11, 1.2, 1174.66, .021, .3);
  }
  const pcm = Buffer.alloc(44 + count * 4);
  pcm.write('RIFF',0);pcm.writeUInt32LE(pcm.length-8,4);pcm.write('WAVEfmt ',8);pcm.writeUInt32LE(16,16);pcm.writeUInt16LE(1,20);pcm.writeUInt16LE(2,22);pcm.writeUInt32LE(rate,24);pcm.writeUInt32LE(rate*4,28);pcm.writeUInt16LE(4,32);pcm.writeUInt16LE(16,34);pcm.write('data',36);pcm.writeUInt32LE(count*4,40);
  const delay = Math.round(.22 * rate);
  for (let n = 0; n < count; n++) {
    const fade = Math.min(1, n / (rate * .75), (count - n) / (rate * 1.5));
    const l = (left[n] + (n >= delay ? right[n-delay] * .19 : 0)) * fade;
    const r = (right[n] + (n >= delay ? left[n-delay] * .19 : 0)) * fade;
    pcm.writeInt16LE(Math.round(Math.tanh(l * 1.7) * 22000),44+n*4);
    pcm.writeInt16LE(Math.round(Math.tanh(r * 1.7) * 22000),46+n*4);
  }
  await mkdir(path.dirname(destination),{recursive:true});await writeFile(destination,pcm);
  return destination;
}
if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  await createSoundtrack(path.resolve(process.argv[2] || 'media/intro/output/soundtrack.wav'));
  console.log('Created original 36-second stereo instrumental soundtrack.');
}
