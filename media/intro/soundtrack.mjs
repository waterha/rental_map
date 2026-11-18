import { mkdir, writeFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import path from 'node:path'

// Short separated plucks: no sustained pad, bass, kick, detuning or noise bed.
export async function createSoundtrack(destination) {
  const rate = 48000, duration = 40, count = rate * duration;
  const left = new Float32Array(count), right = new Float32Array(count);
  const tau = Math.PI * 2;
  function pluck(start, hz, gain = .10, pan = 0, length = .30) {
    const begin = Math.round(start * rate), samples = Math.round(length * rate);
    const l = Math.sqrt((1 - pan) * .5), r = Math.sqrt((1 + pan) * .5);
    for (let n = 0; n < samples && begin + n < count; n++) {
      const t = n / rate;
      const attack = Math.sin(Math.min(1, t / .006) * Math.PI / 2);
      const release = Math.min(1, (length - t) / .06);
      const envelope = attack * Math.exp(-t * 16) * release * release;
      const value = (Math.sin(tau * hz * t) + .16 * Math.sin(tau * hz * 2 * t) + .035 * Math.sin(tau * hz * 3 * t)) * envelope * gain;
      left[begin + n] += value * l; right[begin + n] += value * r;
    }
  }
  const notes = [523.251, 587.330, 659.255, 783.991, 880.000, 1046.502];
  const phrases = [
    [.4,[0,2,3]],[3.1,[2,0]],[4.7,[2,3,4]],[8,[3,2,0]],
    [11,[0,2,4]],[14,[3,2,1]],[17.2,[2,3,5]],[20.6,[4,3,2]],
    [24,[0,2,3]],[27.4,[2,4,3]],[31.3,[3,4,5]],[34,[2,3]],
    [36,[0,2,4]],[38.5,[3,2,0]],
  ];
  for (const [start, melody] of phrases) melody.forEach((note, index) => {
    pluck(start + index * .43, notes[note], index === 0 ? .105 : .075, (index - 1) * .22);
  });
  for (const cue of [6.7, 16.5, 24.8, 28.8, 35.65, 36.9]) {
    pluck(cue, 1046.502, .038, -.15, .15);
    pluck(cue + .105, 1318.510, .026, .15, .18);
  }
  const pcm = Buffer.alloc(44 + count * 4);
  pcm.write('RIFF',0);pcm.writeUInt32LE(pcm.length-8,4);pcm.write('WAVEfmt ',8);pcm.writeUInt32LE(16,16);pcm.writeUInt16LE(1,20);pcm.writeUInt16LE(2,22);pcm.writeUInt32LE(rate,24);pcm.writeUInt32LE(rate*4,28);pcm.writeUInt16LE(4,32);pcm.writeUInt16LE(16,34);pcm.write('data',36);pcm.writeUInt32LE(count*4,40);
  for (let n = 0; n < count; n++) {
    pcm.writeInt16LE(Math.round(Math.max(-1, Math.min(1, left[n])) * 32767),44+n*4);
    pcm.writeInt16LE(Math.round(Math.max(-1, Math.min(1, right[n])) * 32767),46+n*4);
  }
  await mkdir(path.dirname(destination),{recursive:true});await writeFile(destination,pcm);
  return destination;
}
if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  await createSoundtrack(path.resolve(process.argv[2] || 'media/intro/output/soundtrack-v5.wav'));
  console.log('Created sparse short plucks without sustained bass or drone.');
}
